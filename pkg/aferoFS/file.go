package aferoFS

import (
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/spf13/afero"
)

type file struct {
	afero.File
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.File.Stat()
}

func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	if rd, ok := f.File.(interface {
		ReadDir(n int) ([]fs.DirEntry, error)
	}); ok {
		return rd.ReadDir(n)
	}
	// Fallback if afero.File doesn't directly support ReadDir (unlikely for directories in afero)
	infos, err := f.File.Readdir(n)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	entries := make([]fs.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = fs.FileInfoToDirEntry(info)
	}
	return entries, nil
}

// Interface checks
var (
	_ fs.File        = &file{}
	_ fs.ReadDirFile = &file{}
	_ io.ReaderAt    = &file{}
	_ io.Seeker      = &file{}
)
