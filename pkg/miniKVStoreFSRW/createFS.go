package miniKVStoreFSRW

import (
	"io"
	"io/fs"

	"github.com/je4/filesystem/v4/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	resolver "go.ub.unibas.ch/cloud/miniresolverclient/pkg/miniresolverclient"
)

func NewCreateFSFunc(miniResolverClient *resolver.MiniResolver, domain, vfs, dir string, closer []io.Closer, readOnly bool, logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, baseFolder string, readOnly bool) (fs.FS, error) {
		return NewFS(miniResolverClient, domain, vfs, dir, closer, readOnly, logger)
	}
}
