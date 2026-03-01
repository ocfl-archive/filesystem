# vfsrw

`vfsrw` (Virtual File System Read-Write) is a virtual file system layer that provides a unified interface for various backend file systems using a consistent naming scheme.

## Key Features

- **`vfs:/` Prefix**: All paths within the virtual filesystem are prefixed with `vfs:/`, followed by the name of the specific backend (e.g., `vfs:/my-storage/path/to/file`).
- **Unified Interface**: Implements `writefs.FullFS`, providing consistent access to read and write operations across diverse storage systems.
- **Configurable Backends**: Supports multiple backends like OS file systems, S3, SFTP, RemoteFS, and more.
- **Dynamic Configuration**: Backends can be configured via a map of configurations.

## Path Scheme

Paths are structured as follows:
`vfs:/<backend-name>/<file-path>`

For example, if you have a backend named `data` and you want to access `images/photo.jpg`, the full path would be:
`vfs:/data/images/photo.jpg`

## Configuration

`vfsrw` is typically initialized using a `Config` object, which is a map where keys are backend names and values are `VFS` configuration objects.

```go
type VFS struct {
	Name        string       `toml:"name"`
	Type        string       `toml:"type"`
	ReadOnly    bool         `toml:"readonly"`
	S3          *S3          `toml:"s3,omitempty"`
	OS          *OS          `toml:"os,omitempty"`
	SFTP        *SFTP        `toml:"sftp,omitempty"`
	Remote      *Remote      `toml:"remote,omitempty"`
	MiniKVStore *MiniKVStore `toml:"minikvstore,omitempty"`
	Web         *Web         `toml:"web,omitempty"`
}
```

## Backend Types

Supported backend types include:
- `os`: Local operating system filesystem.
- `s3`: Amazon S3 or compatible storage (e.g., MinIO).
- `sftp`: Secure File Transfer Protocol.
- `remote`: Custom remote filesystem protocol.
- `minikvstore`: MiniKVStore-based filesystem.
- `web`: Web/HTTP-based read-only (mostly) filesystem.

Each backend may also support `zipAsFolder` capability if configured.

## Backend Configuration Details

Each backend type has its own configuration structure.

### `os` - Local OS Filesystem
Used for local file system access.
- `BaseDir`: The root directory of the backend.
- `ZipAsFolderCache`: Size of the cache for `zipAsFolder` capability.

```toml
[local]
name = "local"
type = "os"

[local.os]
basedir = "/tmp/data"
zipasfoldercache = 10
```

### `s3` - Amazon S3 or Compatible
Used for S3-compatible storage like AWS S3 or MinIO.
- `AccessKeyID`: Credentials.
- `SecretAccessKey`: Credentials.
- `Endpoint`: S3 endpoint URL.
- `Region`: S3 region.
- `UseSSL`: Enable/disable SSL.
- `Debug`: Enable debug logging.
- `CAPEM`: Custom CA certificate in PEM format.
- `BaseUrl`: Base URL for the S3 service.
- `ZipAsFolderCache`: Size of the cache for `zipAsFolder` capability.
- `DNSNetwork`: Network for DNS resolution (e.g., "tcp").
- `DNSAddress`: Address for DNS resolution.

```toml
[s3-storage]
name = "s3-storage"
type = "s3"

[s3-storage.s3]
accesskeyid = "AKIA..."
secretaccesskey = "..."
endpoint = "s3.amazonaws.com"
region = "us-east-1"
usessl = true
```

### `sftp` - Secure File Transfer Protocol
Used for remote file access via SFTP.
- `Address`: SFTP server address (`host:port`).
- `User`: SFTP username.
- `Password`: SFTP password.
- `PrivateKey`: Paths to private key files.
- `KnownHosts`: List of known hosts.
- `BaseDir`: Root directory on the SFTP server.
- `Sessions`: Number of concurrent sessions.
- `ZipAsFolderCache`: Size of the cache for `zipAsFolder` capability.

```toml
[remote-sftp]
name = "remote-sftp"
type = "sftp"

[remote-sftp.sftp]
address = "sftp.example.com:22"
user = "myuser"
password = "..."
basedir = "/home/myuser"
```

### `remote` - RemoteFS Protocol
A custom gRPC-based remote filesystem protocol.
- `Address`: RemoteFS server address.
- `ClientTLS`: TLS configuration for the client.
- `BaseDir`: Root directory on the remote server.
- `JWTKey`: JWT key for authentication.
- `InsecureSkipVerify`: Skip TLS certificate verification.
- `ca`: Custom CA certificates.

```toml
[remotefs]
name = "remotefs"
type = "remote"

[remotefs.remote]
address = "remotefs.example.com:50051"
jwtkey = "..."
```

### `minikvstore` - MiniKVStore-based Filesystem
A filesystem backend using MiniKVStore for storage.
- `ResolverAddr`: MiniResolver address.
- `ResolverTimeout`: Timeout for resolution.
- `ResolverNotFoundTimeout`: Timeout for not found results.
- `Domain`: Domain in MiniKVStore.
- `ClientTLS`: TLS configuration.
- `Dir`: Base directory within the store.

```toml
[kv-store]
name = "kv-store"
type = "minikvstore"

[kv-store.minikvstore]
resolveraddr = "resolver.example.com"
domain = "mydomain"
```

### `web` - Web/HTTP Filesystem
A read-only filesystem backend over HTTP.
- `baseuri`: Base URL of the web server.
- `header`: Custom HTTP headers to include in requests.
- `tls_insecure_skip_verify`: Skip TLS certificate verification.

```toml
[web-site]
name = "web-site"
type = "web"

[web-site.web]
baseuri = "https://example.com/files/"
```

## Usage

```go
import (
	"github.com/je4/filesystem/v3/pkg/vfsrw"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// Example configuration
cfg := vfsrw.Config{
	"local": &vfsrw.VFS{
		Type: "os",
		OS: &vfsrw.OS{BaseDir: "/tmp/data"},
	},
}

// Initialize VFS
vfs, err := vfsrw.NewFS(cfg, logger)
if err != nil {
	// handle error
}

// Open a file
f, err := vfs.Open("vfs:/local/myfile.txt")
```
