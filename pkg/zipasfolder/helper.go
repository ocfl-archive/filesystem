package zipasfolder

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/je4/filesystem/v4/pkg/writefs"
)

func incRef(fsys fs.FS) {
	if refFS, ok := fsys.(IsRefCountFS); ok {
		refFS.IncRef()
	}
}

func decRef(fsys fs.FS) {
	if refFS, ok := fsys.(IsRefCountFS); ok {
		refFS.DecRef()
	}
}

func getRefCount(fsys fs.FS) int32 {
	if refFS, ok := fsys.(IsRefCountFS); ok {
		rc := refFS.RefCount()
		return rc
	}
	return 0
}

func isLocked(fsys fs.FS) bool {
	if lockedFS, ok := fsys.(writefs.IsLockedFS); ok {
		return lockedFS.IsLocked()
	}
	return false
}

type IsClosedFS interface {
	IsClosed() bool
}

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
