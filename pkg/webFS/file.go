package webFS

import (
	"io/fs"
	"net/http"
	"path"
	"time"
)

type file struct {
	*http.Response
}

func (f *file) Name() string {
	basename := path.Base(f.Response.Request.URL.RawPath)
	return basename
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
	if f.Response.Body == nil {
		return 0, fs.ErrInvalid
	}
	return f.Response.Body.Read(p)
}

func (f *file) Close() error {
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
