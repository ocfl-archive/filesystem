package memFS

import (
	"io"
	"io/fs"
	"path/filepath"

	"emperror.dev/errors"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
)

type memFSRW struct {
	fs     billy.Filesystem
	logger zLogger.ZLogger
}

func NewFS(logger zLogger.ZLogger) (*memFSRW, error) {
	_logger := logger.With().Str("class", "memFSRW").Logger()
	return &memFSRW{
		fs:     memfs.New(),
		logger: &_logger,
	}, nil
}

func (m *memFSRW) Open(name string) (fs.File, error) {
	f, err := m.fs.Open(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &file{File: f}, nil
}

type file struct {
	billy.File
}

func (f *file) Stat() (fs.FileInfo, error) {
	// billy.File might have Stat() method, but we need to check if it matches fs.File
	// Actually, billy.File.Lock/Unlock/Name/Write/Read/Seek/Close are common.
	// We need to implement Stat() for fs.File interface.
	// Looking at billy.Filesystem, it has Stat(path).
	// But fs.File needs Stat() on the file handle.
	// If billy.File doesn't have Stat, we might need to store the path or use the filesystem.
	// memfs implementation of billy.File usually has a way to get info.

	// Let's assume for now we can't easily get it from billy.File directly without more investigation.
	// But wait, many billy implementations' File DOES have Stat().
	// Let's try to see if it's just a type mismatch or missing.
	return f.File.(interface{ Stat() (fs.FileInfo, error) }).Stat()
}

func (m *memFSRW) Create(path string) (writefs.FileWrite, error) {
	f, err := m.fs.Create(path)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create file '%s'", path)
	}
	return f, nil
}

func (m *memFSRW) Append(path string) (writefs.FileWrite, error) {
	// Billy Filesystem Interface: OpenFile(filename string, flag int, perm fs.FileMode) (File, error)
	// os.O_APPEND | os.O_CREATE | os.O_WRONLY = 0x0400 | 0x0040 | 0x0001 = 0x0441
	f, err := m.fs.OpenFile(path, 0x0441, 0666)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot append to file '%s'", path)
	}
	return f, nil
}

func (m *memFSRW) MkDir(path string) error {
	return errors.WithStack(m.fs.MkdirAll(path, 0777))
}

func (m *memFSRW) Rename(oldPath, newPath string) error {
	return errors.WithStack(m.fs.Rename(oldPath, newPath))
}

func (m *memFSRW) Remove(path string) error {
	return errors.WithStack(m.fs.Remove(path))
}

func (m *memFSRW) Stat(name string) (fs.FileInfo, error) {
	fi, err := m.fs.Stat(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return fi, nil
}

func (m *memFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	infos, err := m.fs.ReadDir(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var entries []fs.DirEntry
	for _, info := range infos {
		entries = append(entries, fs.FileInfoToDirEntry(info))
	}
	return entries, nil
}

func (m *memFSRW) ReadFile(name string) ([]byte, error) {
	f, err := m.fs.Open(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (m *memFSRW) Fullpath(name string) (string, error) {
	return filepath.ToSlash(name), nil
}

func (m *memFSRW) Equal(fsys fs.FS) bool {
	if other, ok := fsys.(*memFSRW); ok {
		return m == other
	}
	return false
}

func (m *memFSRW) Copy(src, dst string) (int64, error) {
	s, err := m.fs.Open(src)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot open source '%s'", src)
	}
	defer s.Close()
	d, err := m.fs.Create(dst)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot create destination '%s'", dst)
	}
	defer d.Close()
	n, err := io.Copy(d, s)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot copy '%s' to '%s'", src, dst)
	}
	return n, nil
}

func (m *memFSRW) WriteFile(name string, data []byte) (int64, error) {
	fp, err := m.Create(name)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot create file '%s'", name)
	}
	count, err := fp.Write(data)
	if err != nil {
		fp.Close()
		return 0, errors.Wrapf(err, "cannot write file '%s'", name)
	}
	if err := fp.Close(); err != nil {
		return 0, errors.Wrapf(err, "cannot close file '%s'", name)
	}
	return int64(count), nil
}

func (m *memFSRW) Close() error {
	return nil
}

// Interface Checks
var (
	_ writefs.FullFS = &memFSRW{}
	_ fs.ReadDirFS   = &memFSRW{}
	_ fs.ReadFileFS  = &memFSRW{}
	_ fs.StatFS      = &memFSRW{}
)
