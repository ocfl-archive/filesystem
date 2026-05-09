package zipfsrw

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v4/pkg/writefs"
	"github.com/je4/filesystem/v4/pkg/zipfs"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// NewZipFSRW creates a new ReadWriteFS
// If the file does not exist, it will be created on the first write operation.
// If the file exists, it will be opened and read.
// Changes will be written to an additional file and then renamed to the original file.
// additional writers will added via io.MultiWriter
// additional writers will not be closed
func NewFSFile(baseFS fs.FS, path string, noCompression, readOnly bool, logger zLogger.ZLogger, writers ...io.Writer) (fs.FS, error) {
	if readOnly {
		return zipfs.NewFSFile(baseFS, path, logger)
	}
	writerPath := path

	var zipFS zipfs.OpenRawZipFS

	if xfs, err := zipfs.NewFSFile(baseFS, path, logger); err != nil {
		if !errors.Is(errors.Cause(err), fs.ErrNotExist) {
			return nil, errors.Wrapf(err, "cannot open zip file '%s'", path)
		}
	} else {
		zipFS = xfs
		writerPath = path + ".tmp"
	}

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

	zipFSRWBase, err := NewFS(mainWriter, zipFS, noCompression, fmt.Sprintf("fsFile(%v/%s)", baseFS, path), readOnly, logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create zipFSRW")
	}

	return &fsFile{
		zipFSRW:     zipFSRWBase,
		path:        path,
		writerPath:  writerPath,
		baseFS:      baseFS,
		newZIPFP:    zipFP,
		zipFPBuffer: zipFPBuffer,
		zipFS:       zipFS,
	}, nil
}

type fsFile struct {
	*zipFSRW
	baseFS fs.FS
	//fp          fs.File
	newZIPFP    writefs.FileWrite
	zipFPBuffer *bufio.Writer
	path        string
	writerPath  string
	zipFS       zipfs.OpenRawZipFS
}

func (file *fsFile) String() string {
	return fmt.Sprintf("fsFile(%v/%s)", file.baseFS, file.path)
}

func (file *fsFile) Close() error {
	var errs = []error{}
	if err := file.zipFSRW.Close(); err != nil {
		errs = append(errs, errors.WithStack(err))
	}
	if err := file.zipFPBuffer.Flush(); err != nil {
		errs = append(errs, errors.WithStack(err))
	}
	if err := file.newZIPFP.Close(); err != nil {
		errs = append(errs, errors.WithStack(err))
	}
	if file.zipFS != nil {
		if err := writefs.Close(file.zipFS); err != nil {
			errs = append(errs, errors.WithStack(err))
		}
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
