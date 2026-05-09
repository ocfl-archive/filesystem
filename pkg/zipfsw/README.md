# zipfsrw

`zipfsrw` is a wrapper for linear writing of ZIP files using the `writefs.FullFS` interface.

## Features

- **Linear Writing**: Supports only linear write operations (`Create`).
- **Write-Only**: Does not support reading from the ZIP file during the write process.
- **On-Close Finalization**: The ZIP structure is finalized when `Close()` is called.

## Usage

```go
import (
	"os"
	"github.com/je4/filesystem/v4/pkg/zipfsw"
)

// Open a ZIP file for writing
f, _ := os.Create("archive.zip")
zfsrw, err := zipfsw.NewFS(
    f, 
    false, // no-compression
    "archive.zip", 
    logger,
)

// Write a file into the ZIP
w, _ := zfsrw.Create("in-zip.txt")
w.Write([]byte("Hello inside ZIP!"))
w.Close()

// Finalize and close the ZIP
zfsrw.Close()
```
