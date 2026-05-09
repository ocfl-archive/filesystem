//go:build !windows

package vfsrw

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/osfsrw"
)

func AddLocal(fSys fs.FS) error {
	vfs, ok := fSys.(*vFSRW)
	if !ok {
		return errors.Errorf("fSys is not a *vFSRW, but %T", fSys)
	}

	osFS, err := osfsrw.NewFS("/", false, vfs.logger)
	if err != nil {
		return errors.Wrap(err, "cannot create osfsrw for /")
	}
	vfs.AddFS("root", osFS)
	vfs.logger.Info().Msg("added local / to vfs as root")
	return nil
}

func pathToVFSPath(pathStr string) (string, string, error) {
	if pathStr == "" {
		return "", "", errors.New("path is empty")
	}
	if !filepath.IsAbs(pathStr) {
		wd, err := os.Getwd()
		if err != nil {
			return "", "", errors.Wrap(err, "cannot get current working directory")
		}
		pathStr = filepath.Join(wd, pathStr)
	}
	pathStr = filepath.ToSlash(filepath.Clean(pathStr))
	pathStr = strings.TrimPrefix(pathStr, "/")
	return "root", pathStr, nil
}
