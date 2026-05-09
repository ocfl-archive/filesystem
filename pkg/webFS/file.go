package webFS

import (
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"emperror.dev/errors"
)

type file struct {
	*http.Response
	fs     *webFSRW
	url    string
	offset int64
	mu     sync.Mutex
}

func (f *file) Name() string {
	u, err := url.Parse(f.url)
	if err != nil {
		return path.Base(f.url)
	}
	return path.Base(u.Path)
}

func (f *file) Size() int64 {
	return f.Response.ContentLength
}

func (f *file) Mode() fs.FileMode {
	return 0444 // read-only
}

func (f *file) ModTime() time.Time {
	lm := f.Response.Header.Get("Last-Modified")
	if lm == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC1123, lm)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (f *file) IsDir() bool {
	return false
}

func (f *file) Sys() any {
	return nil
}

func (f *file) Read(p []byte) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Response.Body == nil {
		return 0, fs.ErrInvalid
	}
	n, err = f.Response.Body.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset + offset
	case io.SeekEnd:
		newOffset = f.Response.ContentLength + offset
	default:
		return 0, fs.ErrInvalid
	}

	if newOffset < 0 {
		return 0, errors.New("negative offset")
	}

	if newOffset == f.offset {
		return newOffset, nil
	}

	// Close current body and reopen with range
	if f.Response.Body != nil {
		f.Response.Body.Close()
	}

	resp, err := f.fs.queryRange(f.url, newOffset, -1)
	if err != nil {
		return 0, err
	}
	f.Response = resp
	f.offset = newOffset
	return f.offset, nil
}

func (f *file) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	// Use a separate request for ReadAt to not interfere with the current offset
	resp, err := f.fs.queryRange(f.url, off, off+int64(len(p))-1)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return io.ReadFull(resp.Body, p)
}

func (f *file) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Response.Body == nil {
		return nil
	}
	return f.Response.Body.Close()
}

func (f *file) Stat() (info fs.FileInfo, err error) {
	return f, nil
}

var _ fs.File = (*file)(nil)
var _ fs.FileInfo = (*file)(nil)
var _ io.Seeker = (*file)(nil)
var _ io.ReaderAt = (*file)(nil)
