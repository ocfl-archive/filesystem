package zipfs

import (
	"io/fs"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

func NewCreateFSFunc(logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, zipFile string, readOnly bool) (fs.FS, error) {
		zipFile = clearPath(zipFile)
		if zipFile == "" {
			return nil, errors.New("empty zip path")
		}
		parts := strings.Split(zipFile, "/")
		var baseFolder string
		var fileName string
		if len(parts) == 1 {
			baseFolder = ""
			fileName = parts[0]
		} else {
			baseFolder = strings.Join(parts[:len(parts)-1], "/")
			fileName = parts[len(parts)-1]
		}
		baseFS, err := f.Get(baseFolder, readOnly)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get base filesystem for '%s'", zipFile)
		}
		zipFS, err := NewFSFile(baseFS, fileName, logger)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create zip filesystem for '%s'", zipFile)
		}
		return zipFS, nil
	}
}
