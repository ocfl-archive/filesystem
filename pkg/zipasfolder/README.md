# zipasfolder

`zipasfolder` is a wrapper that treats ZIP files within a base filesystem as if they were folders.

## Features

- **Transparent Expansion**: When navigating a path, if a `.zip` file is encountered, the wrapper treats it as a directory.
- **Dynamic Mounting**: Automatically "mounts" ZIP files when accessed.
- **Cache Management**: Uses a cache for opened ZIP files to improve performance.
- **Read/Write Support**: If the underlying filesystem and ZIP implementation support it, writes can be performed directly into ZIP files within the folder structure.

## Example

Given a file system with `data/archive.zip` containing `file.txt`.
With `zipasfolder`, you can access it as: `data/archive.zip/file.txt`.

## Usage

```go
import (
	"github.com/je4/filesystem/v3/pkg/zipasfolder"
)

// Wrap an existing filesystem
fsys, err := zipasfolder.NewFS(baseFS, 10, time.Minute, false, logger)
if err != nil {
	// handle error
}

// Access a file inside a ZIP
f, err := fsys.Open("path/to/myarchive.zip/content.txt")
```
