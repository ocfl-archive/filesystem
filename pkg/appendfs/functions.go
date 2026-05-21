package appendfs

import (
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

type nopCloser struct{}

func (nopCloser) Close() error {
	return nil
}

// Sub returns a sub-filesystem starting at the specified path.
// It returns the new filesystem, an io.Closer, and an error.
// The io.Closer is important for releasing resources if the underlying
// filesystem requires it. If the original fsys implements io.Closer,
// it is returned. Otherwise, a nopCloser is returned so the caller
// can always safely call Close().
// An example of a filesystem that requires a closer is zipasfolder.
func Sub(fsys FS, path string) (FS, io.Closer, error) {
	if fsys == nil {
		return nil, nil, errors.New("fsys is nil")
	}
	newFS, err := writefs.SubCreate(fsys, path)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "subfs %v/%s", fsys, path)
	}
	newAppendFS, ok := newFS.(FS)
	if !ok {
		return nil, nil, errors.Errorf("subfs %v/%s is not a FS", fsys, path)
	}
	var closer io.Closer
	if c, ok := fsys.(io.Closer); ok {
		closer = c
	} else {
		closer = nopCloser{}
	}
	return newAppendFS, closer, nil
}

// EnsureFS checks if the given fsys implements the appendfs.FS interface.
// The interface requires support for Create and Mkdir in addition to the standard fs.FS methods.
func EnsureFS(fsys fs.FS) (FS, error) {
	if fsys == nil {
		return nil, errors.New("fsys is nil")
	}
	if sfs, ok := fsys.(FS); ok {
		return sfs, nil
	}
	return nil, errors.Errorf("filesystem %T does not implement appendfs.FS (must support Create and Mkdir)", fsys)
}
