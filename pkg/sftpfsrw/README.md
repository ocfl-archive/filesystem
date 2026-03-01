# sftpfsrw

`sftpfsrw` provides a `writefs.FullFS` implementation over the SFTP protocol.

## Features

- **Secure Transfer**: Communicates over SSH to provide secure file access.
- **Full Write Capability**: Supports `Create`, `Append`, `MkDir`, `Remove`, and `Rename`.
- **Session Management**: Manages SFTP sessions for efficient communication.
- **Path Isolation**: Paths are relative to a specified base directory on the remote server.

## Usage

```go
import (
	"github.com/je4/filesystem/v3/pkg/sftpfsrw"
	"golang.org/x/crypto/ssh"
)

// SSH configuration
sshConfig := &ssh.ClientConfig{
	User: "username",
	Auth: []ssh.AuthMethod{
		ssh.Password("password"),
	},
}

// Initialize SFTP filesystem
fsys, err := sftpfsrw.NewFS(
    "example.com:22", 
    sshConfig, 
    "/remote/data", 
    5, // number of sessions
    false, // readOnly
    logger,
)
```
