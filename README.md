# Filesystem Library

This Go library provides advanced filesystem features not available in the standard library's `io/fs`. It introduces write support via `writefs` and a configurable virtual filesystem layer via `vfsrw`.

## Core Features

- **Write Support (`writefs`)**: Extends `io/fs` with standard interfaces for write operations like `Create`, `Append`, `MkDir`, `Rename`, `Remove`, `WriteFile`, and `Copy`.
- **Virtual File System (`vfsrw`)**: A unified layer that maps multiple backend filesystems into a single hierarchy using a `vfs:/<backend>/<path>` scheme.
- **Transparent ZIP Access**: Support for treating ZIP files as folders within other filesystems.

## Available Filesystems

This library includes various storage backends and wrappers:

- **Local Storage**: [osfsrw](./pkg/osfsrw/README.md) - Standard OS filesystem.
- **Object Storage**: [s3fsrw](./pkg/s3fsrw/README.md) - Amazon S3, MinIO.
- **Remote Protocols**: [sftpfsrw](./pkg/sftpfsrw/README.md), [remotefs](./pkg/remotefs/README.md), [webFS](./pkg/webFS/README.md).
- **Archives**: [zipfsw](./pkg/zipfsw/README.md) (Write-only), [zipfsrw](./pkg/zipfsrw/README.md) (R/W), [zipfs](./pkg/zipfs/README.md) (RO), [zipasfolder](./pkg/zipasfolder/README.md).
- **Specialized**: [miniKVStoreFSRW](./pkg/miniKVStoreFSRW/README.md), [mountFS](./pkg/mountFS/README.md), [aferoFS](./pkg/aferoFS/README.md).

## Getting Started

### Installation

```bash
go get github.com/je4/filesystem/v3
```

### Basic Usage with `vfsrw`

The `vfsrw` package allows you to configure multiple backends and access them via a common URI-like scheme.

```go
import (
	"github.com/je4/filesystem/v3/pkg/vfsrw"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// Define backend configuration
cfg := vfsrw.Config{
	"data": &vfsrw.VFS{
		Type: "os",
		OS: &vfsrw.OS{BaseDir: "/var/lib/mydata"},
	},
	"backup": &vfsrw.VFS{
		Type: "s3",
		S3: &vfsrw.S3{
			Endpoint: "s3.amazonaws.com",
			AccessKeyID: "YOUR_KEY",
			SecretAccessKey: "YOUR_SECRET",
			Region: "us-east-1",
			UseSSL: true,
		},
	},
}

// Initialize VFS
vfs, err := vfsrw.NewFS(cfg, logger)
if err != nil {
	// handle error
}

// Automatically add local drives (Windows) or root (Unix) to the VFS. 
// Passing nil enables transparent ZIP access with default settings.
if err := vfsrw.AddLocal(vfs, nil); err != nil {
	// handle error
}

// Or enable transparent ZIP access with custom settings (e.g. cache size)
zCfg := &vfsrw.ZipAsFolder{
	Enabled:   true,
	CacheSize: 20,
}
if err := vfsrw.AddLocal(vfs, zCfg); err != nil {
	// handle error
}

// Open a file for reading
f, err := vfs.Open("vfs:/data/logs/app.log")

// Create a new file (if backend supports writing)
fw, err := vfs.Create("vfs:/data/config.json")
```

For more details on each package, see the [Packages Overview](./pkg/README.md).