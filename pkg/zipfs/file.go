package zipfs

import (
	"io"
	"io/fs"
	"sync"

	"emperror.dev/errors"
)

func NewFile(info fs.FileInfo, rc io.Reader, closeFunc func()) fs.File {
	return &file{
		reader:    rc,
		fi:        info,
		closeFunc: closeFunc,
	}
}

type file struct {
	reader    io.Reader
	fi        fs.FileInfo
	closeFunc func()
	closeOnce sync.Once
	closeErr  error
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := f.reader.(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	return 0, errors.Wrapf(fs.ErrInvalid, "file does not support seeking")
}

func (f *file) Read(bytes []byte) (int, error) {
	return f.reader.Read(bytes)
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.fi, nil
}

func (f *file) Close() error {
	f.closeOnce.Do(func() {
		if f.closeFunc != nil {
			defer f.closeFunc()
		}
		if closer, ok := f.reader.(io.Closer); ok {
			f.closeErr = closer.Close()
		}
	})
	return f.closeErr
}

var (
	_ fs.File           = (*file)(nil)
	_ io.ReadSeekCloser = (*file)(nil)
)
