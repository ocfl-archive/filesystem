package main

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/configs"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var configfile = flag.String("config", "", "location of toml configuration file")
var path = flag.String("path", "", "path to vfs")

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	flag.Parse()

	var cfgFS fs.FS
	var cfgFile string
	if *configfile != "" {
		cfgFS = os.DirFS(filepath.Dir(*configfile))
		cfgFile = filepath.Base(*configfile)
	} else {
		cfgFS = configs.ConfigFS
		cfgFile = "vfsstat.toml"
	}
	conf := &VFSStatConfig{}
	if err := LoadVFSStatConfig(cfgFS, cfgFile, conf); err != nil {
		log.Fatal().Msgf("cannot load toml from [%v] %s: %v", cfgFS, cfgFile, err)
	}
	var logger zLogger.ZLogger = &log.Logger
	vfs, err := vfsrw.NewFS(conf.VFS, logger)
	if err != nil {
		logger.Panic().Err(err).Msg("cannot create vfs")
	}
	defer func() {
		if err := vfs.Close(); err != nil {
			logger.Error().Err(err).Msg("cannot close vfs")
		}
	}()
	fi, err := vfs.Stat(*path)
	if err != nil {
		logger.Fatal().Err(err).Msgf("cannot stat path %s", *path)
	}
	logger.Info().Str("path", *path).Msgf("vfs stat: %s (%d bytes, mode: %s, modtime: %s)",
		fi.Name(), fi.Size(), fi.Mode(), fi.ModTime().Format("2006-01-02 15:04:05"))
}
