package zipfsw

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

// NewZipFSRW creates a new ReadWriteFS
// If the file does not exist, it will be created on the first write operation.
// If the file exists, it will be opened and read.
// Changes will be written to an additional file and then renamed to the original file.
// additional writers will added via io.MultiWriter
// additional writers will not be closed
func NewFSFile(baseFS fs.FS, path string, noCompression bool, logger zLogger.ZLogger, writers ...io.Writer) (fs.FS, error) {
	writerPath := path

	// create new file
	zipFP, err := writefs.Create(baseFS, writerPath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zip file '%s'", writerPath)
	}
	// add a buffer to the file
	zipFPBuffer := bufio.NewWriterSize(zipFP, 1024*1024)

	var mainWriter io.Writer
	if len(writers) > 0 {
		mainWriter = io.MultiWriter(append(writers, zipFPBuffer)...)
	} else {
		mainWriter = zipFPBuffer
	}

	zipFSRWBase, err := NewFS(
		mainWriter,
		true,
		noCompression,
		fmt.Sprintf("fsFile(%v/%s)", baseFS, path),
		nil,
		nil,
		logger,
	)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create zipFSRW")
	}

	var zFS *zipFSW
	switch t := zipFSRWBase.(type) {
	case *zipFSW:
		zFS = t
	case *zipFSWCloser:
		zFS = t.zipFSW
	default:
		return nil, errors.Errorf("unsupported type %T for zipFSRWBase", zipFSRWBase)
	}

	return &fsFile{
		zipFSW:      zFS,
		path:        path,
		writerPath:  writerPath,
		baseFS:      baseFS,
		newZIPFP:    zipFP,
		zipFPBuffer: zipFPBuffer,
	}, nil
}

type fsFile struct {
	*zipFSW
	baseFS      fs.FS
	newZIPFP    writefs.FileWrite
	zipFPBuffer *bufio.Writer
	path        string
	writerPath  string
}

func (file *fsFile) String() string {
	return fmt.Sprintf("fsFile(%v/%s)", file.baseFS, file.path)
}

func (file *fsFile) Open(name string) (fs.File, error) {
	return nil, errors.WithStack(fs.ErrNotExist)
}

func (file *fsFile) Close() error {
	var errs = []error{}
	if err := file.zipFSW.Close(); err != nil {
		errs = append(errs, errors.WithStack(err))
	}
	if err := file.zipFPBuffer.Flush(); err != nil {
		errs = append(errs, errors.WithStack(err))
	}
	if err := file.newZIPFP.Close(); err != nil {
		errs = append(errs, errors.WithStack(err))
	}

	if file.HasChanged() && file.path != file.writerPath {
		if err := writefs.Remove(file.baseFS, file.path); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
		if err := writefs.Rename(file.baseFS, file.writerPath, file.path); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
	}

	if len(errs) > 0 {
		return errors.Combine(errs...)
	}

	return nil
}

var (
	_ fs.FS = (*fsFile)(nil)
)
