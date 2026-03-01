# remotefs

`remotefs` provides a client implementation for interacting with a custom remote filesystem service.

## Features

- **Network Access**: Connects to a remote server over HTTP/HTTPS with TLS support.
- **Authentication**: Uses JWT for secure communication.
- **Remote Operations**: Proxies file system requests to the server, including `Open`, `Stat`, `Create`, `Remove`, and `Rename`.
- **Sub-systems**: Supports sub-filesystem access within the remote system.

## Usage

```go
import (
	"github.com/je4/filesystem/v3/pkg/remotefs"
)

// Initialize remote FS client
fsys, err := remotefs.NewFS(
    tlsConfig, 
    "https://api.example.com", 
    "/base/dir", 
    "vfs-name", 
    nil, // closers
    "jwt-signing-key", 
    false, // read-only
    logger,
)
```
