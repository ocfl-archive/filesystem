# Packages Overview

The `pkg/` directory contains various filesystem implementations and wrappers that extend Go's standard `io/fs` to support both read and write operations.

## Core Features

- [**writefs**](./writefs/README.md): Defines the core interfaces for write-enabled filesystems.
- [**vfsrw**](./vfsrw/README.md): Provides a virtual filesystem with a `vfs:/` prefix for accessing multiple configurable backends.

## Filesystem Implementations

These packages provide actual storage backend implementations:

- [**osfsrw**](./osfsrw/README.md): Local OS filesystem.
- [**s3fsrw**](./s3fsrw/README.md): Amazon S3 and compatible object storage.
- [**sftpfsrw**](./sftpfsrw/README.md): SFTP (Secure File Transfer Protocol).
- [**remotefs**](./remotefs/README.md): Custom remote filesystem service.
- [**miniKVStoreFSRW**](./miniKVStoreFSRW/README.md): MiniKVStore-backed storage.
- [**webFS**](./webFS/README.md): Web/HTTP-based filesystem.
- [**zipfsrw**](./zipfsrw/README.md): Read/Write support for ZIP files.
- [**zipfsw**](./zipfsw/README.md): Write-only support for ZIP files (sequential creation).

## Filesystem Wrappers

These packages provide additional functionality by wrapping other filesystems:

- [**mountFS**](./mountFS/README.md): Combine multiple filesystems into a single unified hierarchy.
- [**zipasfolder**](./zipasfolder/README.md): Treats ZIP files inside another filesystem as if they were directories.
- [**zipfs**](./zipfs/README.md): Simple read-only ZIP filesystem.
