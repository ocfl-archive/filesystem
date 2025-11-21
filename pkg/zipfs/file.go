package zipfs

import (
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/writefs"
)

func NewFile[E io.Reader](info fs.FileInfo, rc E, mutex *writefs.Mutex) fs.File {
	return &file[E]{
		reader: rc,
		fi:     info,
		mutex:  mutex,
	}
}

type file[E io.Reader] struct {
	reader E
	fi     fs.FileInfo
	mutex  *writefs.Mutex
}

func (f *file[E]) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := any(f.reader).(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	return 0, errors.Wrapf(fs.ErrInvalid, "file does not support seeking")
}

func (f *file[E]) Read(bytes []byte) (int, error) {
	return f.reader.Read(bytes)
}

func (f *file[E]) Stat() (fs.FileInfo, error) {
	return f.fi, nil
}

func (f *file[E]) Close() error {
	defer f.mutex.Unlock()
	if closer, ok := any(f.reader).(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

var (
	_ fs.File           = (*file[io.ReadSeekCloser])(nil)
	_ io.ReadSeekCloser = (*file[io.ReadSeekCloser])(nil)
)
