//go:build windows

package vfsrw

import (
	"io/fs"
	"path/filepath"
	"strings"

	"emperror.dev/errors"
	"github.com/ocfl-archive/filesystem/pkg/osfsrw"
	"golang.org/x/sys/windows"
)

func AddLocal(fSys fs.FS, zConfig *ZipAsFolder) error {
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
			vfs.AddFS(
				driveLetter,
				&VFS{
					Name:        driveLetter,
					Type:        "os",
					ReadOnly:    false,
					ZipAsFolder: zConfig,
				},
				osFS,
			)
			vfs.logger.Info().Msgf("added local drive %s to vfs as %s", drivePath, driveLetter)
		}
	}

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
	/*
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

	*/
	name := strings.ToLower(pathStr[0:1])
	newPath := strings.TrimPrefix(pathStr[2:], "/")
	return name, newPath, nil
}
