package aferoFS

import (
	"io"
	"io/fs"
	"path/filepath"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/spf13/afero"
)

type aferoFSRW struct {
	fs     afero.Fs
	logger zLogger.ZLogger
}

func (m *aferoFSRW) IsEmpty(dir string) (bool, error) {
	entries, err := m.ReadDir(dir)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return len(entries) == 0, nil
}

func (m *aferoFSRW) SubCreate(dir string) (fs.FS, error) {
	if err := m.MkDir(dir); err != nil {
		return nil, errors.Wrapf(err, "cannot create subdirectory '%s'", dir)
	}
	return m.Sub(dir)
}

func (m *aferoFSRW) RealPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func NewFS(fsys afero.Fs, logger zLogger.ZLogger) (*aferoFSRW, error) {
	return &aferoFSRW{
		fs:     fsys,
		logger: new(logger.With().Str("class", "aferoFSRW").Logger()),
	}, nil
}

func (m *aferoFSRW) Open(name string) (fs.File, error) {
	f, err := m.fs.Open(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &file{File: f}, nil
}

func (m *aferoFSRW) Create(path string) (writefs.FileWrite, error) {
	f, err := m.fs.Create(path)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create file '%s'", path)
	}
	return f, nil
}

func (m *aferoFSRW) Append(path string) (writefs.FileWrite, error) {
	f, err := m.fs.OpenFile(path, 0x0441, 0666) // os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if err != nil {
		return nil, errors.Wrapf(err, "cannot append to file '%s'", path)
	}
	return f, nil
}

func (m *aferoFSRW) MkDir(path string) error {
	return errors.WithStack(m.fs.MkdirAll(path, 0777))
}

func (m *aferoFSRW) Rename(oldPath, newPath string) error {
	return errors.WithStack(m.fs.Rename(oldPath, newPath))
}

func (m *aferoFSRW) Remove(path string) error {
	return errors.WithStack(m.fs.Remove(path))
}

func (m *aferoFSRW) Stat(name string) (fs.FileInfo, error) {
	fi, err := m.fs.Stat(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return fi, nil
}

func (m *aferoFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	f, err := m.fs.Open(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	infos, err := f.Readdir(-1)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var entries []fs.DirEntry
	for _, info := range infos {
		di := fs.FileInfoToDirEntry(info)
		entries = append(entries, di)
	}
	return entries, nil
}

func (m *aferoFSRW) ReadFile(name string) ([]byte, error) {
	return afero.ReadFile(m.fs, name)
}

func (m *aferoFSRW) Fullpath(name string) (string, error) {
	return filepath.ToSlash(name), nil
}

func (m *aferoFSRW) Equal(fsys fs.FS) bool {
	if other, ok := fsys.(*aferoFSRW); ok {
		return m == other
	}
	return false
}

func (m *aferoFSRW) Copy(src, dst string) (int64, error) {
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

func (m *aferoFSRW) WriteFile(name string, data []byte) (int64, error) {
	err := afero.WriteFile(m.fs, name, data, 0666)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot write file '%s'", name)
	}
	return int64(len(data)), nil
}

func (m *aferoFSRW) IsWriteable(path string) bool {
	return true
}

func (m *aferoFSRW) GetAfero() afero.Fs {
	return m.fs
}

func (m *aferoFSRW) Close() error {
	return nil
}

func (m *aferoFSRW) Sub(dir string) (fs.FS, error) {
	return writefs.NewSubFS(m, dir)
}

// Interface Checks
var (
	_ writefs.FullFS = &aferoFSRW{}
	_ fs.ReadDirFS   = &aferoFSRW{}
	_ fs.ReadFileFS  = &aferoFSRW{}
	_ fs.StatFS      = &aferoFSRW{}
)
