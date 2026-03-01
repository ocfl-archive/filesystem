# webFS

`webFS` provides a read-only (with limited write) `io/fs` implementation over HTTP/HTTPS.

## Features

- **HTTP/HTTPS Access**: Access remote resources via standard web protocols.
- **Header Support**: Can include custom headers for authentication or session management.
- **Insecure Mode**: Supports skipping TLS verification if needed.

## Usage

```go
import (
    "github.com/je4/filesystem/v3/pkg/webFS"
)

// Initialize Web filesystem
fsys, err := webFS.NewFS(
    "https://api.example.com",
    headers, // map[string][]string
    true, // insecure skip verify
    logger,
)
```
