// Package zipfsrw provides a functionality to create and update content of a zip file
package zipfsrw

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/ocfl-archive/filesystem/pkg/zipfs"
	"golang.org/x/exp/slices"
)

func NewFS(writer io.Writer, zipFS zipfs.OpenRawZipFS, noCompression bool, name string, readOnly bool, logger zLogger.ZLogger) (*zipFSRW, error) {
	zipWriter := zip.NewWriter(writer)
	return &zipFSRW{
		zipReader:     zipFS,
		zipWriter:     zipWriter,
		readOnly:      readOnly,
		newFiles:      []string{},
		noCompression: noCompression,
		name:          name,
		logger:        logger,
	}, nil
}

type zipFSRW struct {
	zipReader     zipfs.OpenRawZipFS
	zipWriter     *zip.Writer
	newFiles      []string
	noCompression bool
	name          string
	logger        zLogger.ZLogger
	readOnly      bool
}

func (zfsrw *zipFSRW) Copy(src, dst string) (int64, error) {
	if zfsrw.readOnly {
		return 0, errors.New("read only zip filesystem")
	}
	fp, header, err := zfsrw.zipReader.OpenRaw(src)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot open file '%s'", src)
	}
	defer fp.Close()
	newHeader := *header
	newHeader.Name = dst
	w, err := zfsrw.zipWriter.CreateRaw(&newHeader)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot create file '%s'", dst)
	}
	return io.Copy(w, fp)
}

func (zfsrw *zipFSRW) Append(path string) (writefs.FileWrite, error) {
	if zfsrw.readOnly {
		return nil, errors.New("read-only filesystem")
	}
	src, err := zfsrw.zipReader.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s'", path)
	}
	dst, err := zfsrw.Create(path)
	if err != nil {
		src.Close()
		return nil, errors.Wrapf(err, "cannot create file '%s'", path)
	}
	if _, err := io.Copy(dst, src); err != nil {
		src.Close()
		dst.Close()
		return nil, errors.Wrapf(err, "cannot copy file '%s'", path)
	}
	return dst, nil
}

func (zfsrw *zipFSRW) Remove(path string) error {
	zfsrw.newFiles = append(zfsrw.newFiles, path)
	return nil
}

func (zfsrw *zipFSRW) Fullpath(name string) (string, error) {
	return name, nil
}

func (zfsrw *zipFSRW) Equal(fsys fs.FS) bool {
	if zfs2, ok := fsys.(*zipFSRW); ok {
		return zfsrw.name == zfs2.name
	}
	return false
}

func (zfsrw *zipFSRW) ReadFile(name string) ([]byte, error) {
	if zfsrw.zipReader != nil {
		return fs.ReadFile(zfsrw.zipReader, name)
	}
	return nil, fmt.Errorf("write only zip file")
}

func (zfsrw *zipFSRW) Stat(name string) (fs.FileInfo, error) {
	if zfsrw.zipReader != nil {
		return fs.Stat(zfsrw.zipReader, name)
	}
	return nil, fmt.Errorf("write only zip file")
}

func (zfsrw *zipFSRW) String() string {
	return fmt.Sprintf("zipFSRW(%s)", zfsrw.name)
}

func (zfsrw *zipFSRW) HasChanged() bool {
	return len(zfsrw.newFiles) > 0
}

func (zfsrw *zipFSRW) Close() error {
	var errs = []error{}

	// copy old compressed files to new zip file
	if zfsrw.zipReader != nil {
		zipReader := zfsrw.zipReader.GetZipReader()
		for _, f := range zipReader.File {
			if !slices.Contains(zfsrw.newFiles, f.Name) {
				rc, err := f.OpenRaw()
				if err != nil {
					errs = append(errs, err)
					break
				}
				w, err := zfsrw.zipWriter.CreateRaw(&f.FileHeader)
				if err != nil {
					errs = append(errs, err)
					break
				}
				if _, err := io.Copy(w, rc); err != nil {
					errs = append(errs, err)
					break
				}
			}
		}
	}

	if err := zfsrw.zipWriter.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Combine(errs...)
}

func (zfsrw *zipFSRW) Open(name string) (fs.File, error) {
	name = clearPath(name)
	if slices.Contains(zfsrw.newFiles, name) {
		return nil, errors.Wrapf(fs.ErrPermission, "file '%s' is not yet written to disk", name)
	}
	if zfsrw.zipReader == nil {
		return nil, errors.WithStack(fs.ErrNotExist)
	}
	fp, err := zfsrw.zipReader.Open(name)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s'", name)
	}
	return fp, nil
}

func (zfsrw *zipFSRW) Create(path string) (writefs.FileWrite, error) {
	if zfsrw.readOnly {
		return nil, errors.New("read-only filesystem")
	}
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

func (zfsrw *zipFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	if zfsrw.zipReader == nil {
		return []fs.DirEntry{}, nil
	}
	return fs.ReadDir(zfsrw.zipReader, name)
}

func (zfsrw *zipFSRW) MkDir(string) error {
	if zfsrw.readOnly {
		return errors.New("read-only filesystem")
	}
	return nil
}

var (
	_ fmt.Stringer       = (*zipFSRW)(nil)
	_ writefs.CopyFS     = (*zipFSRW)(nil)
	_ writefs.CreateFS   = (*zipFSRW)(nil)
	_ writefs.AppendFS   = (*zipFSRW)(nil)
	_ writefs.MkDirFS    = (*zipFSRW)(nil)
	_ writefs.RemoveFS   = (*zipFSRW)(nil)
	_ writefs.CloseFS    = (*zipFSRW)(nil)
	_ writefs.FullpathFS = (*zipFSRW)(nil)
	_ writefs.EqualFS    = (*zipFSRW)(nil)
	_ fs.FS              = (*zipFSRW)(nil)
	_ fs.ReadDirFS       = (*zipFSRW)(nil)
	_ fs.ReadFileFS      = (*zipFSRW)(nil)
	_ fs.StatFS          = (*zipFSRW)(nil)
)
