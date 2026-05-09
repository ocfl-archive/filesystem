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

func NewFS(writer io.Writer, noCompression bool, name string, logger zLogger.ZLogger) (*zipFSW, error) {
	zipWriter := zip.NewWriter(writer)
	return &zipFSW{
		zipWriter:     zipWriter,
		newFiles:      []string{},
		noCompression: noCompression,
		name:          name,
		logger:        logger,
	}, nil
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
