package aferoFS

import (
	"io/fs"
	"strings"

	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/spf13/afero"
)

func NewCreateFSFunc(logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f *writefs.Factory, baseFolder string, readOnly bool) (fs.FS, error) {
		var aferoFS afero.Fs
		if strings.HasPrefix(baseFolder, "mem://") {
			aferoFS = afero.NewMemMapFs()
		} else {
			folder := strings.TrimPrefix(baseFolder, "file://")
			aferoFS = afero.NewBasePathFs(afero.NewOsFs(), folder)
		}
		if readOnly {
			aferoFS = afero.NewReadOnlyFs(aferoFS)
		}
		return NewFS(aferoFS, logger)
	}
}
