package memFS

import (
	"io/fs"

	"emperror.dev/errors"
	"github.com/go-git/go-billy/v5"
)

type file struct {
	billy.File
}

func (f *file) Stat() (fs.FileInfo, error) {
	if s, ok := f.File.(interface{ Stat() (fs.FileInfo, error) }); ok {
		return s.Stat()
	}
	return nil, errors.Wrapf(fs.ErrInvalid, "file %s does not support Stat()", f.Name())
}
