package miniKVStoreFSRW

import (
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"go.ub.unibas.ch/cloud/miniresolver/v2/pkg/resolver"
	"io"
	"io/fs"
)

func NewCreateFSFunc(miniResolverClient *resolver.MiniResolver, domain, vfs, dir string, closer []io.Closer, readOnly bool, logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f *writefs.Factory, baseFolder string, readOnly bool) (fs.FS, error) {
		return NewFS(miniResolverClient, domain, vfs, dir, closer, readOnly, logger)
	}
}
