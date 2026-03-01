# zipfsrw

`zipfsrw` is a wrapper for reading and writing ZIP files using the `writefs.FullFS` interface.

## Features

- **ZIP Management**: Efficiently handles reading from and writing to ZIP archives.
- **Write Operations**: Supports `Create`, `Append`, and `Copy`. Note that many ZIP write operations require rewriting the archive.
- **On-Close Writing**: Changes are often buffered and finalized when `Close()` is called.
- **Transparent Access**: Presents the contents of a ZIP file as a regular filesystem.

## Usage

```go
import (
	"os"
	"github.com/je4/filesystem/v3/pkg/zipfsrw"
)

// Open a ZIP file for writing
f, _ := os.Create("archive.zip")
zfsrw, err := zipfsrw.NewFS(
    f, 
    rawZipFS, 
    false, // compression
    "archive.zip", 
    false, // read-only
    logger,
)

// Write a file into the ZIP
w, _ := zfsrw.Create("in-zip.txt")
w.Write([]byte("Hello inside ZIP!"))
w.Close()

// Finalize and close the ZIP
zfsrw.Close()
```
