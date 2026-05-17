//go:build !windows

package vfsrw

import (
	"io/fs"
	"path/filepath"
	"strings"

	"emperror.dev/errors"
	"github.com/ocfl-archive/filesystem/pkg/osfsrw"
)

func AddLocal(fSys fs.FS, zConfig *ZipAsFolder) error {
	vfs, ok := fSys.(*vFSRW)
	if !ok {
		return errors.Errorf("fSys is not a *vFSRW, but %T", fSys)
	}

	osFS, err := osfsrw.NewFS("/", false, vfs.logger)
	if err != nil {
		return errors.Wrap(err, "cannot create osfsrw for /")
	}
	vfs.AddFS("root", zConfig, osFS)
	vfs.logger.Info().Msg("added local / to vfs as root")
	return nil
}

func pathToVFSPath(pathStr string) (string, string, error) {
	pathStr = filepath.ToSlash(filepath.Clean(pathStr))
	var err error
	if !filepath.IsAbs(pathStr) {
		pathStr, err = filepath.Abs(pathStr)
		if err != nil {
			return "", "", errors.Wrap(err, "cannot get absolute path")
		}
		pathStr = filepath.ToSlash(pathStr)
	}
	pathStr = strings.TrimPrefix(pathStr, "/")
	return "root", pathStr, nil
}
