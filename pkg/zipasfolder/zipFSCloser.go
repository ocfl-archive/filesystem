package zipasfolder

import (
	"io"
	"io/fs"
	"sync/atomic"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/zipfs"
)

// NewZipFSCloser creates a new zipFSCloser which wraps a zip filesystem
// and the underlying zip file. It ensures the zip file is closed when the
// filesystem's reference count reaches zero and Close is called.
func NewZipFSCloser(zipFile fs.File, filename string, logger zLogger.ZLogger) (fs.FS, error) {
	readerAt, ok := zipFile.(io.ReaderAt)
	if !ok {
		return nil, errors.New("zipFile does not implement io.ReaderAt")
	}
	zstat, err := zipFile.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "cannot stat zip file")
	}
	zfs, err := zipfs.NewFS(readerAt, zstat.Size(), filename, logger)
	return &zipFSCloser{
		FS:      zfs,
		zipFile: zipFile,
		logger:  logger,
	}, nil
}

type zipFSCloser struct {
	fs.FS
	zipFile fs.File
	logger  zLogger.ZLogger
	closed  atomic.Bool
}

func (zipFS *zipFSCloser) IsClosed() bool {
	return zipFS.closed.Load()
}

func (zipFS *zipFSCloser) Stat(name string) (fs.FileInfo, error) {
	statFS, ok := zipFS.FS.(fs.StatFS)
	if !ok {
		return nil, errors.New("s3FSRW does not implement StatFS")
	}
	return statFS.Stat(name)
}

func (zipFS *zipFSCloser) ReadDir(name string) ([]fs.DirEntry, error) {
	readDirFS, ok := zipFS.FS.(fs.ReadDirFS)
	if !ok {
		return nil, errors.New("s3FSRW does not implement ReadDirFS")
	}
	return readDirFS.ReadDir(name)
}

// IsRefCountFS defines an interface for filesystems with reference counting.
type IsRefCountFS interface {
	fs.FS
	IncRef()
	DecRef()
	RefCount() int32
}

func (zipFS *zipFSCloser) IncRef() {
	if refFS, ok := zipFS.FS.(IsRefCountFS); ok {
		refFS.IncRef()
	}
}

func (zipFS *zipFSCloser) DecRef() {
	if refFS, ok := zipFS.FS.(IsRefCountFS); ok {
		refFS.DecRef()
	}
}

func (zipFS *zipFSCloser) RefCount() int32 {
	if refFS, ok := zipFS.FS.(IsRefCountFS); ok {
		return refFS.RefCount()
	}
	return 0
}

func (zipFS *zipFSCloser) Close() error {
	zipFS.closed.Store(true)
	return errors.WithStack(zipFS.zipFile.Close())
}

var (
	_ fs.FS        = &zipFSCloser{}
	_ fs.ReadDirFS = &zipFSCloser{}
	_ fs.StatFS    = &zipFSCloser{}
	_ IsRefCountFS = &zipFSCloser{}
)
