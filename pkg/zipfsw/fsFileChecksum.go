package zipfsw

import (
	"fmt"
	"io"
	"io/fs"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

// NewZipFSRW creates a new ReadWriteFS
// If the file does not exist, it will be created on the first write operation.
// If the file exists, it will be opened and read.
// Changes will be written to an additional file and then renamed to the original file.
func NewFSFileChecksums(baseFS fs.FS, path string, noCompression bool, algs []checksum.DigestAlgorithm, logger zLogger.ZLogger, writers ...io.Writer) (fs.FS, error) {
	newpath := path

	csWriter, err := checksum.NewChecksumWriter(algs)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create checksum writer for '%s'", newpath)
	}

	mainFS, err := NewFSFile(baseFS, newpath, noCompression, logger, append(writers, csWriter)...)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zip file FS '%s'", newpath)
	}

	return &fsFileChecksums{
		fsFile:   mainFS.(*fsFile),
		csWriter: csWriter,
		csAlgs:   algs,
	}, nil
}

type fsFileChecksums struct {
	*fsFile
	csAlgs   []checksum.DigestAlgorithm
	csWriter *checksum.ChecksumWriter
}

func (zfsrw *fsFileChecksums) String() string {
	return fmt.Sprintf("fsFileChecksums(%v/%s)", zfsrw.baseFS, zfsrw.path)
}

func (zfsrw *fsFileChecksums) Open(name string) (fs.File, error) {
	return nil, errors.WithStack(fs.ErrNotExist)
}

func (zfsrw *fsFileChecksums) Close() error {
	var errs = []error{}

	if err := zfsrw.fsFile.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := zfsrw.csWriter.Close(); err != nil {
		errs = append(errs, err)
	}
	if zfsrw.HasChanged() {
		checksums, err := zfsrw.csWriter.GetChecksums()
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) == 0 {
			for alg, cs := range checksums {
				sideCar := fmt.Sprintf("%s.%s", zfsrw.path, strings.ToLower(string(alg)))
				if _, err := writefs.WriteFile(zfsrw.baseFS, sideCar, []byte(fmt.Sprintf("%s *%s", cs, zfsrw.path))); err != nil {
					errs = append(errs, errors.Wrapf(err, "cannot write sidecar file '%s'", sideCar))
				}
			}
		}
	}

	return errors.WithStack(errors.Combine(errs...))
}

var (
	_ fs.FS = (*fsFileChecksums)(nil)
)
