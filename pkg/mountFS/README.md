# mountFS

`mountFS` is a filesystem wrapper that allows mounting multiple `io/fs.FS` filesystems at specific paths within a single virtual root.

## Features

- **Multi-mount Point**: Mount different filesystem implementations at any path.
- **Unified Hierarchy**: Access multiple storage systems through a single filesystem object.
- **Dynamic Mounting**: Add or remove mounts at runtime.

## Usage

```go
import (
    "github.com/je4/filesystem/v3/pkg/mountFS"
)

// Initialize an empty mount filesystem
mfs := mountFS.NewFS(logger)

// Mount an OS filesystem at /local
mfs.Mount("/local", localOSFS)

// Mount an S3 bucket at /s3
mfs.Mount("/s3", s3FS)

// Access files across mounts
f, _ := mfs.Open("/local/file.txt")
f2, _ := mfs.Open("/s3/image.png")
```
