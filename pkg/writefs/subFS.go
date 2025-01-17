package writefs

import (
	"emperror.dev/errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

type subFS struct {
	fsys fs.FS
	dir  string
}

func Sub(fsys fs.FS, dir string) (fs.FS, error) {
	return &subFS{
		fsys: fsys,
		dir:  dir,
	}, nil
}

func (sfs *subFS) Copy(dst, src string) (int64, error) {
	if copyFS, ok := sfs.fsys.(CopyFS); ok {
		return copyFS.Copy(filepath.ToSlash(filepath.Join(sfs.dir, dst)), filepath.ToSlash(filepath.Join(sfs.dir, src)))
	}
	return _copy(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, dst)), filepath.ToSlash(filepath.Join(sfs.dir, src)))
}

func (sfs *subFS) Append(path string) (FileWrite, error) {
	if appendFS, ok := sfs.fsys.(AppendFS); ok {
		return appendFS.Append(filepath.ToSlash(filepath.Join(sfs.dir, path)))
	}
	return nil, errors.Wrap(ErrNotImplemented, "Append")
}

func (sfs *subFS) Close() error {
	return nil
}

func (sfs *subFS) WriteFile(name string, data []byte) (int64, error) {
	if writeFileFS, ok := sfs.fsys.(WriteFileFS); ok {
		return writeFileFS.WriteFile(filepath.ToSlash(filepath.Join(sfs.dir, name)), data)
	}
	return writeFile(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, name)), data)
}

func (sfs *subFS) Equal(fsys fs.FS) bool {
	if equalFS, ok := fsys.(EqualFS); ok {
		return equalFS.Equal(sfs.fsys)
	}
	return false
}

func (sfs *subFS) Fullpath(name string) (string, error) {
	return Fullpath(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, name)))
}

func (sfs *subFS) String() string {
	return fmt.Sprintf("subFS(%v/%s)", sfs.fsys, sfs.dir)
}

func (sfs *subFS) Rename(oldPath, newPath string) error {
	return Rename(
		sfs.fsys,
		filepath.ToSlash(filepath.Join(sfs.dir, oldPath)),
		filepath.ToSlash(filepath.Join(sfs.dir, newPath)),
	)
}

func (sfs *subFS) Remove(path string) error {
	return Remove(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, path)))
}

func (sfs *subFS) Open(name string) (fs.File, error) {
	return sfs.fsys.Open(filepath.ToSlash(filepath.Join(sfs.dir, name)))
}

func (sfs *subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, name)))
}

func (sfs *subFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, name)))
}

func (sfs *subFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, name)))
}

func (sfs *subFS) Sub(dir string) (fs.FS, error) {
	return Sub(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, dir)))
}

func (sfs *subFS) Create(path string) (FileWrite, error) {
	return Create(sfs.fsys, filepath.ToSlash(filepath.Join(sfs.dir, path)))
}

func (sfs *subFS) MkDir(path string) error {
	mkdirFS, ok := sfs.fsys.(MkDirFS)
	if !ok {
		return errors.New("fs does not support MkDir")
	}
	return mkdirFS.MkDir(filepath.ToSlash(filepath.Join(sfs.dir, path)))
}

var (
	_ FullFS       = &subFS{}
	_ fmt.Stringer = &subFS{}
)
