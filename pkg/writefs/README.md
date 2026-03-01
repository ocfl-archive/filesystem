# writefs

`writefs` is an extension of the Go standard library's `io/fs` package that provides write access to file systems.

## Key Features

- **Write Interfaces**: Extends `io/fs` with interfaces for creating, appending, and managing files and directories.
- **`FullFS` Interface**: A comprehensive interface that combines standard `io/fs` read operations with write operations like `Create`, `Append`, `MkDir`, `Rename`, `Remove`, `Copy`, and `WriteFile`.
- **Interoperability**: Works alongside standard Go `io/fs.FS` implementations while providing additional capabilities when needed.

## Core Interfaces

The package defines several small, focused interfaces for write operations:

- `CreateFS`: Provides a `Create(path string) (FileWrite, error)` method.
- `AppendFS`: Provides an `Append(path string) (FileWrite, error)` method.
- `MkDirFS`: Provides a `MkDir(path string) error` method.
- `RenameFS`: Provides a `Rename(oldPath, newPath string) error` method.
- `RemoveFS`: Provides a `Remove(path string) error` method.
- `WriteFileFS`: Provides a `WriteFile(name string, data []byte) (int64, error)` method.
- `CopyFS`: Provides a `Copy(dst, src string) (int64, error)` method.
- `FullpathFS`: Provides a `Fullpath(name string) (string, error)` method for obtaining the full path of a file.

## `FullFS`

`FullFS` is the most common interface used by this library. It embeds all of the above write-related interfaces plus standard `io/fs` interfaces:

```go
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
```

## Usage

When implementing a new filesystem that supports writing, it should implement the `FullFS` interface or at least the relevant sub-interfaces. Other parts of the library, such as `vfsrw`, use these interfaces to provide a unified way to interact with different storage backends.
