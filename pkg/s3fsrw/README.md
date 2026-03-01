# s3fsrw

`s3fsrw` is a wrapper for Amazon S3-compatible object storage, implementing the `writefs.FullFS` interface.

## Features

- **S3 Compatibility**: Works with Amazon S3, MinIO, and other S3-compatible storage systems.
- **Read/Write Operations**: Supports `Create`, `Append`, `Remove`, `Rename` (via copy and delete), and `WriteFile`.
- **Directory Emulation**: Provides a flat directory structure typical for object storage.
- **Metadata Management**: Handles object metadata and content-types.

## Usage

```go
import (
	"github.com/je4/filesystem/v3/pkg/s3fsrw"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// Initialize with S3 credentials
fsys, err := s3fsrw.NewFS(
    "s3.example.com", 
    "accessKey", 
    "secretKey", 
    "us-east-1", 
    true, // useSSL
    false, // debug
    nil, // tlsConfig
    "", // dnsNetwork
    "", // dnsAddress
    false, // readOnly
    logger,
)
if err != nil {
	// handle error
}

// Write to an S3 object
err = fsys.Create("mybucket/data/file.txt")
```
