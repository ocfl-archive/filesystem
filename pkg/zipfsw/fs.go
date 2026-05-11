// Package zipfsw provides a functionality to create and update content of a zip file
package zipfsw

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v4/pkg/writefs"
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/zLogger"
)

type zipFSWCloser struct {
	*zipFSW
	writer io.WriteCloser
}

func (z *zipFSWCloser) Close() error {
	defer func(writer io.WriteCloser) {
		err := writer.Close()
		if err != nil {
			z.zipFSW.logger.Error().Err(err).Msg("error closing writer")
		}
	}(z.writer)
	return errors.WithStack(z.zipFSW.Close())
}

type ChecksumFunc func(css map[checksum.DigestAlgorithm]string) error

// NewFS creates a new zip file system.
// The writer is used to write the zip file.
// If closeWriter is true and writer implements io.WriteCloser, the writer will be closed when the file system is closed.
// If noCompression is true, the files will be stored without compression.
// The name is used for identification (e.g. in Equal).
// The logger is used for logging errors.
func NewFS(writer io.Writer, closeWriter bool, noCompression bool, name string, algs []checksum.DigestAlgorithm, csFunc ChecksumFunc, logger zLogger.ZLogger) (fs.FS, error) {
	var csWriter *checksum.ChecksumWriter
	var w = writer
	var err error
	if len(algs) > 0 {
		csWriter, err = checksum.NewChecksumWriter(algs, writer)
		if err != nil {
			return nil, errors.Wrap(err, "cannot create checksum writer")
		}
		w = csWriter
	}
	zipWriter := zip.NewWriter(w)
	zFS := &zipFSW{
		closeWriter:   closeWriter,
		writer:        writer,
		zipWriter:     zipWriter,
		newFiles:      []string{},
		noCompression: noCompression,
		name:          name,
		logger:        logger,
		csWriter:      csWriter,
		csFunc:        csFunc,
	}
	return zFS, nil
}

type zipFSW struct {
	zipWriter     *zip.Writer
	newFiles      []string
	noCompression bool
	name          string
	logger        zLogger.ZLogger
	csWriter      *checksum.ChecksumWriter
	closeWriter   bool
	csFunc        ChecksumFunc
	writer        io.Writer
}

func (zfsrw *zipFSW) MkDir(path string) error {
	// in zip file, directories do not exists
	return nil
}

func (zfsrw *zipFSW) Fullpath(name string) (string, error) {
	return name, nil
}

func (zfsrw *zipFSW) Equal(fsys fs.FS) bool {
	if zfs2, ok := fsys.(*zipFSW); ok {
		return zfsrw.name == zfs2.name
	}
	return false
}
func (zfsrw *zipFSW) String() string {
	return fmt.Sprintf("zipFSRW(%s)", zfsrw.name)
}

func (zfsrw *zipFSW) HasChanged() bool {
	return len(zfsrw.newFiles) > 0
}

func (zfsrw *zipFSW) Close() error {
	var errs = []error{}
	var checksums map[checksum.DigestAlgorithm]string
	if err := zfsrw.zipWriter.Close(); err != nil {
		zfsrw.logger.Error().Err(err).Msg("cannot close zip writer")
		errs = append(errs, err)
	}
	if zfsrw.csWriter != nil {
		if err := zfsrw.csWriter.Close(); err != nil {
			zfsrw.logger.Error().Err(err).Msg("cannot close checksum writer")
			errs = append(errs, err)
		} else if zfsrw.csFunc != nil {
			zfsrw.logger.Debug().Msg("checksum writer closed")
			checksums, err = zfsrw.csWriter.GetChecksums()
			if err != nil {
				zfsrw.logger.Error().Err(err).Msg("cannot get checksums")
				errs = append(errs, err)
			}
		}
	}
	if closeWriter, ok := zfsrw.writer.(io.WriteCloser); ok && zfsrw.closeWriter {
		if err := closeWriter.Close(); err != nil {
			zfsrw.logger.Error().Err(err).Msg("cannot close zip writer")
			errs = append(errs, err)
		}
	}
	if len(checksums) > 0 && zfsrw.csFunc != nil {
		zfsrw.logger.Debug().Msgf("checksums: %v", checksums)
		defer func() {
			if err := zfsrw.csFunc(checksums); err != nil {
				errs = append(errs, err)
			}
		}()
	}
	return errors.Combine(errs...)
}

func (zfsrw *zipFSW) Create(path string) (writefs.FileWrite, error) {
	path = clearPath(path)
	header := &zip.FileHeader{
		Name: path,
	}
	if zfsrw.noCompression {
		header.Method = zip.Store
	} else {
		header.Method = zip.Deflate
	}
	fp, err := zfsrw.zipWriter.CreateHeader(header)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create file '%s'", path)
	}
	zfsrw.newFiles = append(zfsrw.newFiles, path)
	return writefs.NewNopWriteCloser(fp), nil
}

func (zfsrw *zipFSW) Open(name string) (fs.File, error) {
	return nil, errors.WithStack(fs.ErrNotExist)
}

var (
	_ fs.FS              = (*zipFSW)(nil)
	_ fmt.Stringer       = (*zipFSW)(nil)
	_ writefs.CreateFS   = (*zipFSW)(nil)
	_ writefs.MkDirFS    = (*zipFSW)(nil)
	_ writefs.CloseFS    = (*zipFSW)(nil)
	_ writefs.FullpathFS = (*zipFSW)(nil)
	_ writefs.EqualFS    = (*zipFSW)(nil)
)
