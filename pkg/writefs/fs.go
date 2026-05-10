package writefs

import "io/fs"

type IsWriteableFS interface {
	IsWriteable(path string) bool
}

type CreateFS interface {
	Create(path string) (FileWrite, error)
}

type AppendFS interface {
	Append(path string) (FileWrite, error)
}

type MkDirFS interface {
	MkDir(path string) error
}

type RenameFS interface {
	Rename(oldPath, newPath string) error
}

type RemoveFS interface {
	Remove(path string) error
}

type CloseFS interface {
	Close() error
}

type WriteFileFS interface {
	WriteFile(name string, data []byte) (int64, error)
}

type CopyFS interface {
	Copy(src, dst string) (int64, error)
}

type FullpathFS interface {
	Fullpath(name string) (string, error)
}

type EqualFS interface {
	Equal(fsys fs.FS) bool
}

type JoinFS interface {
	Join(fsys fs.FS, elems ...string) string
}

type SubFS interface {
	Sub(dir string) (fs.FS, error)
}

type SubCreateFS interface {
	SubCreate(dir string) (fs.FS, error)
}

type RealPathFS interface {
	RealPath(path string) string
}

type FullFS interface {
	CopyFS
	CreateFS
	AppendFS
	MkDirFS
	RenameFS
	RemoveFS
	CloseFS
	WriteFileFS
	FullpathFS
	EqualFS
	SubFS
	SubCreateFS
	RealPathFS
	fs.FS
	fs.ReadDirFS
	fs.ReadFileFS
	fs.StatFS
}
