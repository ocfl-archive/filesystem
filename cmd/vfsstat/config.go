package main

import (
	"io/fs"
	"os"

	"emperror.dev/errors"
	"github.com/BurntSushi/toml"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
)

type VFSStatConfig struct {
	VFS map[string]*vfsrw.VFS `toml:"vfs"`
}

func LoadVFSStatConfig(fSys fs.FS, fp string, conf *VFSStatConfig) error {
	if _, err := fs.Stat(fSys, fp); err != nil {
		path, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "cannot get current working directory")
		}
		fSys = os.DirFS(path)
		fp = "vfsstat.toml"
	}
	data, err := fs.ReadFile(fSys, fp)
	if err != nil {
		return errors.Wrapf(err, "cannot read file [%v] %s", fSys, fp)
	}
	_, err = toml.Decode(string(data), conf)
	if err != nil {
		return errors.Wrapf(err, "error loading config file %v", fp)
	}
	return nil
}
