# osfsrw

`osfsrw` is a wrapper for the local operating system's filesystem, implementing the `writefs.FullFS` interface.

## Features

- **Read/Write Support**: Fully supports all `writefs` write operations (`Create`, `Append`, `MkDir`, `Rename`, `Remove`, `Copy`, `WriteFile`).
- **Standard Library Interoperability**: Implements `fs.ReadDirFS`, `fs.ReadFileFS`, `fs.StatFS`, and `fs.SubFS`.
- **Read-Only Mode**: Can be initialized in a read-only mode where any write attempt will return an error.
- **Path Isolation**: Paths are relative to a specified base directory.

## Usage

```go
import (
	"github.com/je4/filesystem/v3/pkg/osfsrw"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// Initialize with a base directory
fsys, err := osfsrw.NewFS("/path/to/data", false, logger)
if err != nil {
	// handle error
}

// Write a file
bytesWritten, err := fsys.WriteFile("test.txt", []byte("Hello, OSFS!"))
```
