//go:build windows

package vfsrw

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/osfsrw"
	"golang.org/x/sys/windows"
)

func AddLocal(fSys fs.FS) error {
	vfs, ok := fSys.(*vFSRW)
	if !ok {
		return errors.Errorf("fSys is not a *vFSRW, but %T", fSys)
	}

	drives, err := windows.GetLogicalDrives()
	if err != nil {
		return errors.Wrap(err, "cannot get logical drives")
	}

	for i := range uint32(26) {
		if drives&(1<<i) != 0 {
			driveLetter := string(rune('a' + i))
			drivePath := driveLetter + ":/"
			// wir verwenden driveLetter als Namen im VFS
			osFS, err := osfsrw.NewFS(drivePath, false, vfs.logger)
			if err != nil {
				// Falls ein Laufwerk nicht bereit ist (z.B. CD-ROM ohne Medium), loggen wir es nur oder ignorieren es
				vfs.logger.Error().Err(err).Msgf("cannot create osfsrw for drive %s", drivePath)
				continue
			}
			vfs.AddFS(driveLetter, osFS)
			vfs.logger.Info().Msgf("added local drive %s to vfs as %s", drivePath, driveLetter)
		}
	}

	return nil
}

func pathToVFSPath(pathStr string) (string, string, error) {
	if pathStr == "" {
		return "", "", errors.New("path is empty")
	}
	pathStr = filepath.ToSlash(filepath.Clean(pathStr))
	if !(len(pathStr) > 1 && pathStr[1] == ':') {
		dir, err := os.Getwd()
		if err != nil {
			return "", "", errors.Wrap(err, "cannot get current working directory")
		}
		if !(len(dir) > 1 && dir[1] == ':') {
			return "", "", errors.Errorf("current working directory %s does not start with drive letter and :", dir)
		}
		if pathStr[0] == '/' {
			// pathStr ist absolut
			pathStr = filepath.Join(dir[0:1], pathStr)
		} else {
			// pathStr ist relativ
			pathStr = filepath.Join(dir, pathStr)
		}
		pathStr = filepath.ToSlash(pathStr)
	}
	name := strings.ToLower(pathStr[0:1])
	newPath := strings.TrimPrefix(pathStr[2:], "/")
	return name, newPath, nil
}
