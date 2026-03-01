package zipfs

import (
	"io/fs"
	"path"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"golang.org/x/exp/maps"
)

func NewDirFile(zfs *zipFS, folder string) *dirFile {
	return &dirFile{
		zfs:    zfs,
		folder: strings.Trim(folder, "/") + "/",
	}
}

type dirFile struct {
	zfs    *zipFS
	folder string
}

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	var results = map[string]fs.DirEntry{}
	var count int
	for _, fi := range d.zfs.File {
		if !strings.HasPrefix(fi.Name, d.folder) {
			continue
		}
		de := writefs.NewDirEntry(fi.FileInfo())
		results[de.Name()] = de
		count++
		if count >= n {
			break
		}
	}
	return maps.Values(results), nil
}

func (d *dirFile) Stat() (fs.FileInfo, error) {
	return writefs.NewFileInfoDir(path.Base(d.folder)), nil
}

func (d *dirFile) Read(bytes []byte) (int, error) {
	return 0, errors.New("not implemented")
}

func (d *dirFile) Close() error {
	return nil
}

var _ fs.File = (*dirFile)(nil)
var _ fs.ReadDirFile = (*dirFile)(nil)
