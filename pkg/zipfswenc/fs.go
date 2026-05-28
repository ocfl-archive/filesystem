package zipfswenc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/encrypt"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/appendfs"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/ocfl-archive/filesystem/pkg/zipfsw"
	"github.com/tink-crypto/tink-go/v2/core/registry"
	"github.com/tink-crypto/tink-go/v2/keyset"
)

// NewFSFileEncryptedChecksums creates a new write-only encrypted zip filesystem
func NewFSFileChecksumsEncrypted(baseFS fs.FS, path string, noCompression bool, algs []checksum.DigestAlgorithm, keyUri string, logger zLogger.ZLogger) (fs.FS, error) {
	// create encrypted file
	encFP, err := writefs.Create(baseFS, path+".aes")
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zip file '%s'", path)
	}

	// add a buffer to the file
	newEncFPBuffer := bufio.NewWriterSize(encFP, 1024*1024)

	csEncWriter, err := checksum.NewChecksumWriter(algs, newEncFPBuffer)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create checksum writer for '%s'", path+".aes")
	}

	encWriter, err := encrypt.NewEncryptWriterAESGCM(csEncWriter, []byte(path), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create encrypt writer for '%s'", path+".aes")
	}

	handle := encWriter.GetKeysetHandle()

	// Use zipfsw.NewFSFileChecksums in the background
	zipFS, err := zipfsw.NewFSFileChecksums(baseFS, path, noCompression, algs, logger, encWriter)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create zipFS")
	}

	return &fsFileEncryptedChecksums{
		baseFS:      baseFS,
		path:        path,
		zipFS:       zipFS,
		aad:         []byte(path),
		handle:      handle,
		encWriter:   encWriter,
		keyURI:      keyUri,
		csEncWriter: csEncWriter,
		csEncBuffer: newEncFPBuffer,
		logger:      logger,
	}, nil
}

type fsFileEncryptedChecksums struct {
	baseFS      fs.FS
	path        string
	zipFS       fs.FS
	aad         []byte
	handle      *keyset.Handle
	encWriter   io.Closer
	keyURI      string
	csEncWriter *checksum.ChecksumWriter
	csEncBuffer *bufio.Writer
	logger      zLogger.ZLogger
}

func (zfs *fsFileEncryptedChecksums) MkDir(path string) error {
	return nil
}

func (zfs *fsFileEncryptedChecksums) String() string {
	return fmt.Sprintf("zipfswenc.fsFileEncryptedChecksums(%v/%s)", zfs.baseFS, zfs.path)
}

func (zfs *fsFileEncryptedChecksums) Open(name string) (fs.File, error) {
	return nil, errors.WithStack(fs.ErrNotExist)
}

func (zfs *fsFileEncryptedChecksums) Create(name string) (writefs.FileWrite, error) {
	if cfs, ok := zfs.zipFS.(writefs.CreateFS); ok {
		return cfs.Create(name)
	}
	return nil, errors.Errorf("zipFS does not implement writefs.CreateFS")
}

func (zfs *fsFileEncryptedChecksums) Close() error {
	if zfs == nil {
		return errors.New("cannot close nil fsFileEncryptedChecksums")
	}
	var errs = []error{}

	time.Sleep(1 * time.Second)
	if zfs.zipFS == nil {
		return errors.New("cannot close nil fsFileEncryptedChecksums.zipFS")
	}

	if closer, ok := zfs.zipFS.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if zfs.csEncBuffer == nil {
		return errors.New("cannot close nil fsFileEncryptedChecksums.csEncBuffer")
	}
	if err := zfs.csEncBuffer.Flush(); err != nil {
		errs = append(errs, err)
	}
	if zfs.encWriter == nil {
		return errors.New("cannot flush nil fsFileEncryptedChecksums.encWriter")
	}
	if err := zfs.encWriter.Close(); err != nil {
		errs = append(errs, err)
	}
	if zfs.csEncWriter == nil {
		return errors.New("cannot close nil fsFileEncryptedChecksums.csEncWriter")
	}
	if err := zfs.csEncWriter.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		client, err := registry.GetKMSClient(zfs.keyURI)
		if err != nil {
			return errors.Wrapf(err, "cannot get KMS client for '%s'", zfs.keyURI)
		}
		aead, err := client.GetAEAD(zfs.keyURI)
		if err != nil {
			return errors.Wrapf(err, "cannot get AEAD for entry '%s'", zfs.keyURI)
		}
		keyFileName := zfs.path + ".aes.key.json"
		keyBuf := bytes.NewBuffer(nil)
		wr := keyset.NewBinaryWriter(keyBuf)

		if err := zfs.handle.Write(wr, aead); err != nil {
			return errors.Wrapf(err, "cannot write %s", keyFileName)
		}
		ts := encrypt.KeyStruct{
			EncryptedKey: keyBuf.Bytes(),
			Aad:          zfs.aad,
		}
		jsonBytes, err := json.Marshal(ts)
		if err != nil {
			return errors.Wrapf(err, "cannot marshal %s", keyFileName)
		}
		if _, err := writefs.WriteFile(zfs.baseFS, keyFileName, jsonBytes); err != nil {
			return errors.Wrapf(err, "cannot write %s", keyFileName)
		}

		checksums, err := zfs.csEncWriter.GetChecksums()
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) == 0 {
			for alg, cs := range checksums {
				sideCar := fmt.Sprintf("%s.aes.%s", zfs.path, strings.ToLower(string(alg)))
				wfp, err := writefs.Create(zfs.baseFS, sideCar)
				if err != nil {
					errs = append(errs, errors.Wrapf(err, "cannot create sidecar file '%s'", sideCar))
				}
				if _, err := wfp.Write([]byte(fmt.Sprintf("%s *%s.aes", cs, zfs.path))); err != nil {
					errs = append(errs, errors.Wrapf(err, "cannot write to sidecar file '%s'", sideCar))
				}
				if err := wfp.Close(); err != nil {
					errs = append(errs, errors.Wrapf(err, "cannot close sidecar file '%s'", sideCar))
				}
			}
		}

	}

	return errors.WithStack(errors.Combine(errs...))
}

func (zfs *fsFileEncryptedChecksums) Equal(fsys fs.FS) bool {
	if zfs2, ok := fsys.(*fsFileEncryptedChecksums); ok {
		return zfs.path == zfs2.path
	}
	return false
}

var (
	_ fs.FS            = (*fsFileEncryptedChecksums)(nil)
	_ fmt.Stringer     = (*fsFileEncryptedChecksums)(nil)
	_ writefs.CreateFS = (*fsFileEncryptedChecksums)(nil)
	_ writefs.CloseFS  = (*fsFileEncryptedChecksums)(nil)
	_ writefs.EqualFS  = (*fsFileEncryptedChecksums)(nil)
	_ appendfs.FS      = (*fsFileEncryptedChecksums)(nil)
)
