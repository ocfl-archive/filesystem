package zipfs

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync/atomic"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

type OpenRawZipFS interface {
	fs.FS
	OpenRaw(name string) (fs.File, *zip.FileHeader, error)
	GetZipReader() *zip.Reader
}

// NewFS creates a new fs.FS from a readerAt and size
// it implements fs.FS, fs.ReadDirFS, fs.ReadFileFS, fs.StatFS, fs.SubFS, writefs.IsLockedFS
func NewFS(r io.ReaderAt, size int64, name string, logger zLogger.ZLogger) (*zipFS, error) {
	zipReader, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}

	files := make(map[string]*zip.File, len(zipReader.File))
	dirEntries := make(map[string]map[string]fs.DirEntry)
	dirEntries[""] = make(map[string]fs.DirEntry)

	for _, f := range zipReader.File {
		cleanName := clearPath(f.Name)
		if cleanName == "" {
			continue
		}

		isDir := strings.HasSuffix(f.Name, "/") || f.FileInfo().IsDir()
		if isDir {
			if _, ok := dirEntries[cleanName]; !ok {
				dirEntries[cleanName] = make(map[string]fs.DirEntry)
			}
		} else {
			files[cleanName] = f
		}

		curr := cleanName
		isLeaf := true
		for curr != "" {
			parent := path.Dir(curr)
			if parent == "." {
				parent = ""
			}
			base := path.Base(curr)

			if _, ok := dirEntries[parent]; !ok {
				dirEntries[parent] = make(map[string]fs.DirEntry)
			}

			if isLeaf {
				if isDir {
					dirEntries[parent][base] = writefs.NewDirEntry(f.FileInfo())
				} else {
					dirEntries[parent][base] = writefs.NewDirEntry(f.FileInfo())
				}
				isLeaf = false
			} else {
				if _, exists := dirEntries[parent][base]; !exists {
					dirEntries[parent][base] = writefs.NewDirEntry(writefs.NewFileInfoDir(base))
				}
			}

			curr = parent
		}
	}

	dirs := make(map[string][]fs.DirEntry, len(dirEntries))
	for dirPath, entryMap := range dirEntries {
		entries := make([]fs.DirEntry, 0, len(entryMap))
		for _, entry := range entryMap {
			entries = append(entries, entry)
		}
		slices.SortFunc(entries, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})
		dirs[dirPath] = entries
	}

	return &zipFS{
		reader: zipReader,
		files:  files,
		dirs:   dirs,
		mutex:  writefs.NewMutex(),
		name:   name,
		logger: logger,
	}, nil
}

type zipFS struct {
	reader   *zip.Reader
	files    map[string]*zip.File
	dirs     map[string][]fs.DirEntry
	mutex    *writefs.Mutex
	name     string
	logger   zLogger.ZLogger
	refCount atomic.Int32
}

func (zfs *zipFS) IncRef() {
	zfs.refCount.Add(1)
}

func (zfs *zipFS) DecRef() {
	zfs.refCount.Add(-1)
}

func (zfs *zipFS) RefCount() int32 {
	return zfs.refCount.Load()
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
	return zfs.reader
}

func (zfs *zipFS) Sub(dir string) (fs.FS, error) {
	return writefs.NewSubFS(zfs, dir)
}

func (zfs *zipFS) Stat(name string) (fs.FileInfo, error) {
	return zfs.stat(name)
}

func (zfs *zipFS) stat(name string) (fs.FileInfo, error) {
	name = clearPath(name)
	if name == "" {
		return writefs.NewFileInfoDir("."), nil
	}
	if f, ok := zfs.files[name]; ok {
		return f.FileInfo(), nil
	}
	if _, ok := zfs.dirs[name]; ok {
		return writefs.NewFileInfoDir(path.Base(name)), nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (zfs *zipFS) ReadFile(name string) ([]byte, error) {
	name = clearPath(name)
	f, ok := zfs.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (zfs *zipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = clearPath(name)
	entries, ok := zfs.dirs[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return entries, nil
}

func (zfs *zipFS) Open(name string) (fs.File, error) {
	name = clearPath(name)
	if name == "" {
		entries := zfs.dirs[""]
		return NewDirFile(zfs, "", entries), nil
	}
	if entries, ok := zfs.dirs[name]; ok {
		return NewDirFile(zfs, name, entries), nil
	}
	if f, ok := zfs.files[name]; ok {
		if f.Method == zip.Store {
			w, err := f.OpenRaw()
			if err != nil {
				return nil, errors.Wrapf(err, "failed to open file in raw mode '%s'", name)
			}
			zfs.IncRef()
			return NewFile(f.FileInfo(), w, zfs.DecRef), nil
		}
		w, err := f.Open()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open file '%s'", name)
		}
		zfs.IncRef()
		return NewFile(f.FileInfo(), w, zfs.DecRef), nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (zfs *zipFS) OpenRaw(name string) (fs.File, *zip.FileHeader, error) {
	name = clearPath(name)
	if f, ok := zfs.files[name]; ok {
		w, err := f.OpenRaw()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to open file '%s'", name)
		}
		zfs.IncRef()
		return NewFile(f.FileInfo(), writefs.NewNopReadCloser(w), zfs.DecRef), &f.FileHeader, nil
	}
	return nil, nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
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
	_ fs.SubFS            = (*zipFS)(nil)
)
