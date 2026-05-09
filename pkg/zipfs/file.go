package zipfs

import (
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v4/pkg/writefs"
)

func NewFile[E io.Reader](info fs.FileInfo, rc E, mutex *writefs.Mutex, closeFunc func()) fs.File {
	return &file[E]{
		reader:    rc,
		fi:        info,
		mutex:     mutex,
		closeFunc: closeFunc,
	}
}

type file[E io.Reader] struct {
	reader    E
	fi        fs.FileInfo
	mutex     *writefs.Mutex
	closeFunc func()
}

func (f *file[E]) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := any(f.reader).(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	return 0, errors.Wrapf(fs.ErrInvalid, "file does not support seeking")
}

func (f *file[E]) Read(bytes []byte) (int, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.reader.Read(bytes)
}

func (f *file[E]) Stat() (fs.FileInfo, error) {
	return f.fi, nil
}

func (f *file[E]) Close() error {
	if f.closeFunc != nil {
		defer f.closeFunc()
	}
	if closer, ok := any(f.reader).(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

var (
	_ fs.File           = (*file[io.ReadSeekCloser])(nil)
	_ io.ReadSeekCloser = (*file[io.ReadSeekCloser])(nil)
)
