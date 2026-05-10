// Package zipfsw provides a functionality to create and update content of a zip file
package zipfsw

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v4/pkg/writefs"
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

// NewFS creates a new zip file system.
// The writer is used to writing the zip file.
// If closeWriter is true and writer implements io.WriteCloser, the writer will be closed when the file system is closed.
// If noCompression is true, the files will be stored without compression.
// The name is used for identification (e.g. in Equal).
// The logger is used for logging errors.
func NewFS(writer io.Writer, closeWriter bool, noCompression bool, name string, logger zLogger.ZLogger) (fs.FS, error) {
	zipWriter := zip.NewWriter(writer)
	zFS := &zipFSW{
		zipWriter:     zipWriter,
		newFiles:      []string{},
		noCompression: noCompression,
		name:          name,
		logger:        logger,
	}
	if writeCloser, ok := writer.(io.WriteCloser); ok && closeWriter {
		return &zipFSWCloser{
			zipFSW: zFS,
			writer: writeCloser,
		}, nil
	}
	return zFS, nil
}

type zipFSW struct {
	zipWriter     *zip.Writer
	newFiles      []string
	noCompression bool
	name          string
	logger        zLogger.ZLogger
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
	return zfsrw.zipWriter.Close()
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
	_ writefs.CloseFS    = (*zipFSW)(nil)
	_ writefs.FullpathFS = (*zipFSW)(nil)
	_ writefs.EqualFS    = (*zipFSW)(nil)
)
