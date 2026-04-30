package remotefs

import (
	"crypto/tls"
	"io"
	"io/fs"

	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
)

func NewCreateFSFunc(tlsConfig *tls.Config, addr string, vfs string, closer []io.Closer, jwtKey string, logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, baseFolder string, readOnly bool) (fs.FS, error) {
		return NewFS(tlsConfig, addr, baseFolder, vfs, closer, jwtKey, readOnly, logger)
	}
}
