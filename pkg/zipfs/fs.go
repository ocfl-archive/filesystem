package zipfs

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"golang.org/x/exp/slices"
)

type OpenRawZipFS interface {
	fs.FS
	OpenRaw(name string) (fs.File, *zip.FileHeader, error)
	GetZipReader() *zip.Reader
}

// NewFS creates a new fs.FS from a readerAt and size
// it implements fs.FS, fs.ReadDirFS, fs.ReadFileFS, fs.StatFS, fs.SubFS, basefs.IsLockedFS
func NewFS(r io.ReaderAt, size int64, name string, logger zLogger.ZLogger) (fs *zipFS, err error) {
	zipReader, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	return &zipFS{
		Reader: zipReader,
		mutex:  writefs.NewMutex(),
		name:   name,
		logger: logger,
	}, nil
}

type zipFS struct {
	*zip.Reader
	mutex  *writefs.Mutex
	name   string
	logger zLogger.ZLogger
}

func (zfs *zipFS) Close() error {
	return nil
}

func (zfs *zipFS) WriteFile(name string, data []byte) (int64, error) {
	return 0, errors.New("read only zip filesystem not implemented")
}

func (zfs *zipFS) Fullpath(name string) (string, error) {
	return name, nil
}

func (zfs *zipFS) Equal(fsys fs.FS) bool {
	if zfs2, ok := fsys.(*zipFS); ok {
		return zfs.name == zfs2.name
	}
	return false
}

func (zfs *zipFS) String() string {
	return fmt.Sprintf("zipFS(%s)", zfs.name)
}

func (zfs *zipFS) GetZipReader() *zip.Reader {
	return zfs.Reader
}

/*
func (zfs *zipFS) Sub(dir string) (fs.FS, error) {
	return writefs.Sub(zfs, dir)
}

*/

func (zfs *zipFS) Stat(name string) (fs.FileInfo, error) {
	name = clearPath(name)
	for _, f := range zfs.File {
		if strings.HasPrefix(f.Name, name) && len(f.Name) != len(name) {
			return writefs.NewFileInfoDir(name), nil
		}
		if f.Name == name {
			return f.FileInfo(), nil
		}
	}
	return nil, fs.ErrNotExist
}

func (zfs *zipFS) ReadFile(name string) ([]byte, error) {
	name = clearPath(name)
	for _, f := range zfs.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fs.ErrNotExist
}

func (zfs *zipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = clearPath(name)
	var result []fs.DirEntry
	for _, f := range zfs.File {
		if strings.HasPrefix(f.Name, name) {
			parts := strings.Split(strings.Trim(f.Name[len(name):], "/"), "/")
			if len(parts) == 1 {
				if parts[0] != "" {
					result = append(result, writefs.NewDirEntry(f.FileInfo()))
				}
				continue
			}
			result = append(result, writefs.NewDirEntry(writefs.NewFileInfoDir(parts[0])))
		}
	}
	slices.SortFunc(result, func(i, j fs.DirEntry) int {
		return strings.Compare(i.Name(), j.Name())
	})
	return slices.CompactFunc(result, func(i, j fs.DirEntry) bool {
		return i.Name() == j.Name()
	}), nil

}

func (zfs *zipFS) Open(name string) (fs.File, error) {
	name = clearPath(name)
	for _, f := range zfs.File {
		if f.Name == name {
			if f.Method == zip.Store {
				w, err := f.OpenRaw()
				if err != nil {
					return nil, errors.Wrapf(err, "failed to open file in raw mode '%s'", name)
				}
				if rseeker, ok := w.(io.ReadSeeker); ok {
					zfs.mutex.Lock()
					return NewFile[io.ReadSeeker](f.FileInfo(), rseeker, zfs.mutex), nil
				} else {
					zfs.mutex.Lock()
					return NewFile[io.Reader](f.FileInfo(), w, zfs.mutex), nil
				}
			} else {
				w, err := f.Open()
				if err != nil {
					return nil, errors.Wrapf(err, "failed to open file '%s'", name)
				}
				zfs.mutex.Lock()
				return NewFile[io.Reader](f.FileInfo(), w, zfs.mutex), nil
			}
		}
	}
	return nil, fs.ErrNotExist
}

func (zfs *zipFS) OpenRaw(name string) (fs.File, *zip.FileHeader, error) {
	for _, f := range zfs.File {
		if f.Name == name {
			w, err := f.OpenRaw()
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to open file '%s'", name)
			}
			zfs.mutex.Lock()
			return NewFile(f.FileInfo(), writefs.NewNopReadCloser(w), zfs.mutex), &f.FileHeader, nil
		}
	}
	return nil, nil, fs.ErrNotExist
}

func (zfs *zipFS) IsLocked() bool {
	return zfs.mutex.IsLocked()
}

var (
	_ writefs.IsLockedFS  = (*zipFS)(nil)
	_ OpenRawZipFS        = (*zipFS)(nil)
	_ fmt.Stringer        = (*zipFS)(nil)
	_ writefs.CloseFS     = (*zipFS)(nil)
	_ writefs.WriteFileFS = (*zipFS)(nil)
	_ writefs.FullpathFS  = (*zipFS)(nil)
	_ writefs.EqualFS     = (*zipFS)(nil)
	_ fs.FS               = (*zipFS)(nil)
	_ fs.ReadDirFS        = (*zipFS)(nil)
	_ fs.ReadFileFS       = (*zipFS)(nil)
	_ fs.StatFS           = (*zipFS)(nil)
)
