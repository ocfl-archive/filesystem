package writefs

import "io/fs"

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
	Copy(dst, src string) (int64, error)
}

type FullpathFS interface {
	Fullpath(name string) (string, error)
}

type EqualFS interface {
	Equal(fsys fs.FS) bool
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
	fs.FS
	fs.ReadDirFS
	fs.ReadFileFS
	fs.StatFS
}
