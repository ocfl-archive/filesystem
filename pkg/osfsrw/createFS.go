package osfsrw

import (
	"io/fs"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

func NewCreateFSFunc(logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, baseFolder string, readOnly bool) (fs.FS, error) {
		folder := strings.TrimPrefix(baseFolder, "file://")
		osFS, err := NewFS(folder, readOnly, logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return osFS, nil
	}
}
