package writefs

import (
	"emperror.dev/errors"
	"io"
	"io/fs"
	"path/filepath"
)

var ErrNotImplemented = errors.NewPlain("not implemented")

func Equal(fsys1, fsys2 fs.FS) bool {
	if _fsys1, ok := fsys1.(EqualFS); ok {
		return _fsys1.Equal(fsys2)
	}
	return false
}

func SubFSCreate(fsys fs.FS, path string) (fs.FS, error) {
	if err := MkDir(fsys, path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return nil, errors.Wrapf(err, "cannot create directory '%s'", path)
		}
	}
	return Sub(fsys, path)
}

func MkDir(fsys fs.FS, path string) error {
	if _fsys, ok := fsys.(MkDirFS); ok {
		return _fsys.MkDir(path)
	}
	return errors.Wrapf(fs.ErrInvalid, "fs does not support MkDir")
}

func Rename(fsys fs.FS, oldPath, newPath string) error {
	if _fsys, ok := fsys.(RenameFS); ok {
		return _fsys.Rename(oldPath, newPath)
	}

	if _, ok := fsys.(RemoveFS); !ok {
		return errors.Wrap(ErrNotImplemented, "Cannot Rename: Remove")
	}
	if _, ok := fsys.(CopyFS); !ok {
		return errors.Wrap(ErrNotImplemented, "Cannot Rename: Copy")
	}

	if _, err := Copy(fsys, oldPath, newPath); err != nil {
		return errors.Wrapf(err, "cannot copy '%s' to '%s'", oldPath, newPath)
	}
	if err := Remove(fsys, oldPath); err != nil {
		return errors.Wrapf(err, "cannot remove '%s'", oldPath)
	}
	return nil
}

func Create(fsys fs.FS, path string) (FileWrite, error) {
	if _fsys, ok := fsys.(CreateFS); ok {
		return _fsys.Create(path)
	}
	return nil, errors.Wrap(ErrNotImplemented, "Create")
}

func Append(fsys fs.FS, path string) (FileWrite, error) {
	if _fsys, ok := fsys.(AppendFS); ok {
		return _fsys.Append(path)
	}
	return nil, errors.Wrap(ErrNotImplemented, "Append")
}

func Remove(fsys fs.FS, path string) error {
	if _fsys, ok := fsys.(RemoveFS); ok {
		return _fsys.Remove(path)
	}
	return errors.Wrap(ErrNotImplemented, "Remove")
}

func Close(fsys fs.FS) error {
	if _fsys, ok := fsys.(CloseFS); ok {
		return _fsys.Close()
	}
	return nil
}

func Fullpath(fsys fs.FS, name string) (string, error) {
	if _fsys, ok := fsys.(FullpathFS); ok {
		return _fsys.Fullpath(name)
	}
	return "", errors.Wrap(ErrNotImplemented, "Fullpath")
}

func writeFile(fsys fs.FS, name string, data []byte) (int64, error) {
	fp, err := Create(fsys, name)
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

func WriteFile(fsys fs.FS, name string, data []byte) (int64, error) {
	if _fsys, ok := fsys.(WriteFileFS); ok {
		return _fsys.WriteFile(name, data)
	}
	return writeFile(fsys, name, data)
}

func HasContent(fsys fs.FS) bool {
	entries, err := fs.ReadDir(fsys, "")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() != "" {
			return true
		}
	}
	return false
}

func _copy(fsys fs.FS, src, dst string) (int64, error) {
	var srcFP io.ReadCloser
	var err error
	srcFP, err = fsys.Open(src)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot open source '%s'", src)
	}
	var dstFP io.WriteCloser
	dstFP, err = Create(fsys, dst)
	if err != nil {
		srcFP.Close()
		return 0, errors.Wrapf(err, "cannot open destination '%s'", dst)
	}
	var errs []error

	num, err := io.Copy(dstFP, srcFP)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "cannot copy data"))
	}
	if err := dstFP.Close(); err != nil {
		errs = append(errs, errors.Wrap(err, "cannot close destination"))
	}
	if err := srcFP.Close(); err != nil {
		errs = append(errs, errors.Wrap(err, "cannot close source"))
	}
	if len(errs) > 0 {
		return 0, errors.Wrap(errors.Combine(errs...), "cannot copy files")
	}
	return num, nil
}

func Copy(fsys fs.FS, src, dst string) (int64, error) {
	if _fsys, ok := fsys.(CopyFS); ok {
		return _fsys.Copy(dst, src)
	}
	return _copy(fsys, src, dst)
}

func Join(fsys fs.FS, elems ...string) string {
	if _fsys, ok := fsys.(JoinFS); ok {
		return _fsys.Join(fsys, elems...)
	}
	return filepath.ToSlash(filepath.Join(elems...))
}
