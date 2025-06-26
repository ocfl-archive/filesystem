package mountFS

import (
	"emperror.dev/errors"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

func NewMountFS(base fs.FS) *MountFS {
	return &MountFS{
		FS:     base,
		mounts: make(map[string]fs.FS),
	}
}

type MountFS struct {
	fs.FS
	mounts map[string]fs.FS
}

func (m *MountFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = strings.TrimPrefix(path.Clean(filepath.ToSlash(name)), "/") + "/"
	if name == "" || name == "/" {
		name = "."
	}
	for dir, mount := range m.mounts {
		if strings.HasPrefix(name, dir) {
			relPath := strings.TrimPrefix(name, dir)
			if relPath == "" || relPath == "/" {
				relPath = "."
			}
			// If the path is exactly the mount point, we return the root of the mount
			return fs.ReadDir(mount, ".")
		}
	}
	return fs.ReadDir(m.FS, name)
}

func (m *MountFS) Close() error {
	var errs = []error{}
	for dir, mount := range m.mounts {
		if closer, ok := mount.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, errors.Wrapf(err, "cannot close mount %s - %s", mount, dir))
			}
		}
	}
	if closer, ok := m.FS.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, errors.Wrapf(err, "cannot close base filesystem %s", m.FS))
		}
	}
	return errors.Combine(errs...)
}

func (m *MountFS) Mount(dir string, fsys fs.FS) error {
	dir = strings.TrimPrefix(path.Clean(filepath.ToSlash(dir)), "/") + "/"
	if dir == "" {
		return errors.New("mount directory cannot be empty")
	}
	if len(dir) < 2 {
		return errors.New("mount directory must be at least one character long")
	}
	if m.mounts == nil {
		m.mounts = make(map[string]fs.FS)
	}
	if _, exists := m.mounts[dir]; exists {
		return errors.Errorf("mount point %s already exists", dir)
	}
	m.mounts[dir] = fsys
	return nil
}

func (m *MountFS) Open(name string) (fs.File, error) {
	name = strings.TrimPrefix(path.Clean(filepath.ToSlash(name)), "/")
	for dir, mount := range m.mounts {
		if strings.HasPrefix(name, dir) {
			relPath := strings.TrimPrefix(name, dir)
			if relPath == "" || relPath == "/" {
				// If the path is exactly the mount point, we return the root of the mount
				return mount.Open(".")
			}
			// Open the file relative to the mount point
			return mount.Open(relPath)
		}
	}
	return m.FS.Open(name)
}

var _ fs.FS = (*MountFS)(nil)
var _ fs.ReadDirFS = (*MountFS)(nil)
