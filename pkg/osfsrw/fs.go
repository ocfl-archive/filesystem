package osfsrw

import (
	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func NewFS(dir string, readOnly bool, logger zLogger.ZLogger) (*osFSRW, error) {
	_logger := logger.With().Str("class", "osFSRW").Logger()
	logger = &_logger
	var err error
	if dir == "" || dir == "." {
		dir, err = os.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "cannot get current working directory")
		}
	}
	dir = filepath.ToSlash(dir)
	if strings.HasPrefix(dir, "./") {
		currentDir, err := os.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "cannot get current working directory")
		}
		dir = filepath.Join(currentDir, dir[2:])
	}
	dir = filepath.ToSlash(filepath.Clean(dir))
	stat, err := os.Stat(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot stat directory '%s'", dir)
	}
	if !stat.IsDir() {
		return nil, errors.Errorf("not a directory: %s", dir)
	}

	return &osFSRW{
		dir:      dir,
		readOnly: readOnly,
		logger:   logger,
	}, nil
}

type osFSRW struct {
	dir      string
	logger   zLogger.ZLogger
	readOnly bool
}

func (d *osFSRW) Copy(dst, src string) (int64, error) {
	if d.readOnly {
		return 0, errors.New("read only filesystem")
	}
	fpSrc, err := os.Open(filepath.Join(d.dir, src))
	if err != nil {
		return 0, errors.Wrapf(err, "cannot open file '%s'", src)
	}
	defer fpSrc.Close()
	fpDest, err := os.Create(filepath.Join(d.dir, dst))
	if err != nil {
		return 0, errors.Wrapf(err, "cannot create file '%s'", dst)
	}
	num, err := io.Copy(fpDest, fpSrc)
	if err != nil {
		fpDest.Close()
		return 0, errors.Wrapf(err, "cannot copy file '%s' to '%s'", src, dst)
	}
	if err := fpDest.Close(); err != nil {
		return 0, errors.Wrapf(err, "cannot close file '%s'", dst)
	}
	return num, nil
}

func (d *osFSRW) Equal(fsys fs.FS) bool {
	if ofsys, ok := fsys.(*osFSRW); ok {
		return d.dir == ofsys.dir
	}
	return false
}

func (d *osFSRW) Fullpath(name string) (string, error) {
	return filepath.ToSlash(filepath.Join(d.dir, name)), nil
}

func (d *osFSRW) String() string {
	return "osFSRW(" + d.dir + ")"
}

func (d *osFSRW) Sub(dir string) (fs.FS, error) {
	return NewFS(filepath.Join(d.dir, dir), d.readOnly, d.logger)
}

func (d *osFSRW) Remove(path string) error {
	if d.readOnly {
		return errors.New("read only filesystem")
	}
	return errors.WithStack(os.Remove(filepath.Join(d.dir, path)))
}

func (d *osFSRW) Rename(oldPath, newPath string) error {
	if d.readOnly {
		return errors.New("read only filesystem")
	}
	return errors.WithStack(os.Rename(filepath.Join(d.dir, oldPath), filepath.Join(d.dir, newPath)))
}

func (d *osFSRW) Open(name string) (fs.File, error) {
	fp, err := os.Open(filepath.Join(d.dir, name))
	return fp, errors.WithStack(err)
}

func (d *osFSRW) Stat(name string) (fs.FileInfo, error) {
	fi, err := os.Stat(filepath.Join(d.dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.WithStack(fs.ErrNotExist)
		}
		return nil, errors.WithStack(err)
	}
	return fi, nil
}

func (d *osFSRW) Create(path string) (writefs.FileWrite, error) {
	if d.readOnly {
		return nil, errors.New("read only filesystem")
	}
	fullpath := filepath.Join(d.dir, path)
	dir := filepath.Dir(fullpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, errors.Wrapf(err, "cannot create directory '%s'", dir)
	}
	w, err := os.Create(fullpath)
	return w, errors.Wrapf(err, "cannot create file '%s'", fullpath)
}

func (d *osFSRW) Append(path string) (writefs.FileWrite, error) {
	if d.readOnly {
		return nil, errors.New("read only filesystem")
	}
	fullpath := filepath.Join(d.dir, path)
	w, err := os.OpenFile(fullpath, os.O_APPEND|os.O_WRONLY, 0644)
	return w, errors.Wrapf(err, "cannot create file '%s'", fullpath)
}

func (d *osFSRW) MkDir(path string) error {
	if d.readOnly {
		return errors.New("read only filesystem")
	}
	return errors.WithStack(os.Mkdir(filepath.Join(d.dir, path), 0777))
}

func (d *osFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	de, err := os.ReadDir(filepath.Join(d.dir, name))
	if err != nil && os.IsNotExist(err) {
		return nil, fs.ErrNotExist
	}
	return de, errors.WithStack(err)
}

func (d *osFSRW) ReadFile(name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(d.dir, name))
	return data, errors.WithStack(err)
}

func (d *osFSRW) Close() error {
	return nil
}

func (d *osFSRW) WriteFile(name string, data []byte) (int64, error) {
	if d.readOnly {
		return 0, errors.New("read only filesystem")
	}
	fullpath := filepath.ToSlash(filepath.Join(d.dir, name))
	dir := filepath.Dir(fullpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, errors.Wrapf(err, "cannot create directory '%s'", dir)
	}
	if err := os.WriteFile(fullpath, data, 0644); err != nil {
		return 0, errors.Wrapf(err, "cannot write file '%s'", name)
	}
	return int64(len(data)), nil
}

var (
	_ writefs.FullFS = &osFSRW{}
	_ fs.ReadDirFS   = &osFSRW{}
	_ fs.ReadFileFS  = &osFSRW{}
	_ fs.StatFS      = &osFSRW{}
	_ fs.SubFS       = &osFSRW{}
)
