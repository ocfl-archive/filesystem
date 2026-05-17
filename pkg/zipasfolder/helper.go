package zipasfolder

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

// incRef increases the reference count of a filesystem if it supports it.
func incRef(fsys fs.FS) {
	if refFS, ok := fsys.(IsRefCountFS); ok {
		refFS.IncRef()
	}
}

// decRef decreases the reference count of a filesystem if it supports it.
func decRef(fsys fs.FS) {
	if refFS, ok := fsys.(IsRefCountFS); ok {
		refFS.DecRef()
	}
}

// getRefCount returns the current reference count of a filesystem.
func getRefCount(fsys fs.FS) int32 {
	if refFS, ok := fsys.(IsRefCountFS); ok {
		rc := refFS.RefCount()
		return rc
	}
	return 0
}

// isLocked checks if a filesystem is currently locked (e.g. for write operations).
func isLocked(fsys fs.FS) bool {
	if lockedFS, ok := fsys.(writefs.IsLockedFS); ok {
		return lockedFS.IsLocked()
	}
	return false
}

// IsClosedFS is an interface for filesystems that can report their closed state.
type IsClosedFS interface {
	IsClosed() bool
}

// isClosed checks if a filesystem has already been closed.
func isClosed(fsys fs.FS) bool {
	if closedFS, ok := fsys.(IsClosedFS); ok {
		return closedFS.IsClosed()
	}
	return false
}

func clearPath(path string) string {
	if strings.HasPrefix(path, "vfs:/") {
		path = strings.Trim(filepath.ToSlash(filepath.Clean(path[5:])), "/")
		return "vfs://" + path
	}
	path = strings.Trim(filepath.ToSlash(filepath.Clean(path)), "/")
	if path == "." {
		path = ""
	}
	return path
}
