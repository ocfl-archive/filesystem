# miniKVStoreFSRW

`miniKVStoreFSRW` provides a `writefs.FullFS` implementation backed by a MiniKVStore.

## Features

- **KV Store Backend**: Uses MiniKVStore for storage of files and metadata.
- **Dynamic Resolution**: Supports MiniResolver for locating the storage backend.
- **Full Write Capability**: Implements `Create`, `Append`, `MkDir`, `Remove`, and `Rename`.

## Usage

```go
import (
    "github.com/je4/filesystem/v3/pkg/miniKVStoreFSRW"
)

// Initialize MiniKVStore filesystem
fsys, err := miniKVStoreFSRW.NewFS(
    "resolver-addr:port",
    resolverTimeout,
    notFoundTimeout,
    "domain",
    clientTLSConfig,
    "/base/dir",
    logger,
)
```
