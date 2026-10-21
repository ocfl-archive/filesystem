package zipfs

import (
	"io"
	"io/fs"
	"path"
	"sync"

	"emperror.dev/errors"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

func NewDirFile(zfs *zipFS, folder string, entries []fs.DirEntry) *dirFile {
	if zfs != nil {
		zfs.IncRef()
	}
	return &dirFile{
		zfs:     zfs,
		folder:  folder,
		entries: entries,
	}
}

type dirFile struct {
	zfs       *zipFS
	folder    string
	entries   []fs.DirEntry
	offset    int
	mu        sync.Mutex
	closeOnce sync.Once
}

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.entries == nil {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		if d.offset >= len(d.entries) {
			return nil, nil
		}
		res := d.entries[d.offset:]
		d.offset = len(d.entries)
		return res, nil
	}

	if d.offset >= len(d.entries) {
		return nil, io.EOF
	}

	end := d.offset + n
	if end > len(d.entries) {
		end = len(d.entries)
	}
	res := d.entries[d.offset:end]
	d.offset = end
	return res, nil
}

func (d *dirFile) Stat() (fs.FileInfo, error) {
	name := path.Base(d.folder)
	if name == "." || name == "/" || d.folder == "" {
		name = "."
	}
	return writefs.NewFileInfoDir(name), nil
}

func (d *dirFile) Read(bytes []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.folder, Err: errors.New("is a directory")}
}

func (d *dirFile) Close() error {
	d.closeOnce.Do(func() {
		if d.zfs != nil {
			d.zfs.DecRef()
		}
	})
	return nil
}

var (
	_ fs.File        = (*dirFile)(nil)
	_ fs.ReadDirFile = (*dirFile)(nil)
)
