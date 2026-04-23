package writefs

import (
	"emperror.dev/errors"
	"fmt"
	"io/fs"
	"path"
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

func (sfs *subFS) Copy(src, dst string) (int64, error) {
	if copyFS, ok := sfs.fsys.(CopyFS); ok {
		return copyFS.Copy(path.Join(sfs.dir, src), path.Join(sfs.dir, dst))
	}
	return _copy(sfs.fsys, path.Join(sfs.dir, src), path.Join(sfs.dir, dst))
}

func (sfs *subFS) Append(pathStr string) (FileWrite, error) {
	if appendFS, ok := sfs.fsys.(AppendFS); ok {
		return appendFS.Append(path.Join(sfs.dir, pathStr))
	}
	return nil, errors.Wrap(ErrNotImplemented, "Append")
}

func (sfs *subFS) Close() error {
	return nil
}

func (sfs *subFS) WriteFile(name string, data []byte) (int64, error) {
	if writeFileFS, ok := sfs.fsys.(WriteFileFS); ok {
		return writeFileFS.WriteFile(path.Join(sfs.dir, name), data)
	}
	return writeFile(sfs.fsys, path.Join(sfs.dir, name), data)
}

func (sfs *subFS) Equal(fsys fs.FS) bool {
	if equalFS, ok := fsys.(EqualFS); ok {
		return equalFS.Equal(sfs.fsys)
	}
	return false
}

func (sfs *subFS) Fullpath(name string) (string, error) {
	return Fullpath(sfs.fsys, path.Join(sfs.dir, name))
}

func (sfs *subFS) String() string {
	return fmt.Sprintf("subFS(%v/%s)", sfs.fsys, sfs.dir)
}

func (sfs *subFS) Rename(oldPath, newPath string) error {
	return Rename(
		sfs.fsys,
		path.Join(sfs.dir, oldPath),
		path.Join(sfs.dir, newPath),
	)
}

func (sfs *subFS) Remove(pathStr string) error {
	return Remove(sfs.fsys, path.Join(sfs.dir, pathStr))
}

func (sfs *subFS) Open(name string) (fs.File, error) {
	return sfs.fsys.Open(path.Join(sfs.dir, name))
}

func (sfs *subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(sfs.fsys, path.Join(sfs.dir, name))
}

func (sfs *subFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(sfs.fsys, path.Join(sfs.dir, name))
}

func (sfs *subFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(sfs.fsys, path.Join(sfs.dir, name))
}

func (sfs *subFS) Sub(dir string) (fs.FS, error) {
	return Sub(sfs.fsys, path.Join(sfs.dir, dir))
}

func (sfs *subFS) Create(pathStr string) (FileWrite, error) {
	return Create(sfs.fsys, path.Join(sfs.dir, pathStr))
}

func (sfs *subFS) MkDir(pathStr string) error {
	mkdirFS, ok := sfs.fsys.(MkDirFS)
	if !ok {
		return errors.New("fs does not support MkDir")
	}
	return mkdirFS.MkDir(path.Join(sfs.dir, pathStr))
}

var (
	_ FullFS       = &subFS{}
	_ fmt.Stringer = &subFS{}
)
