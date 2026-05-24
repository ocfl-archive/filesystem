package vfsrw

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/ocfl-archive/filesystem/pkg/zipasfolder"
	"github.com/ocfl-archive/filesystem/pkg/zipfsw"
	"go.ub.unibas.ch/cloud/certloader/v2/pkg/loader"
	resolver "go.ub.unibas.ch/cloud/miniresolverclient/pkg/miniresolverclient"
)

type VFSRW interface {
	fmt.Stringer
	writefs.CopyFS
	writefs.CreateFS
	writefs.MkDirFS
	writefs.RenameFS
	writefs.RemoveFS
	writefs.CloseFS
	writefs.EqualFS
	writefs.IsWriteableFS
	writefs.SubFS
	writefs.SubCreateFS
	writefs.RealPathFS
	writefs.IsEmptyFS
	fs.FS
	fs.ReadDirFS
	fs.ReadFileFS
	fs.StatFS
	AddFS(name string, vfsConfig *VFS, fsys fs.FS)
}

type vfsStruct struct {
	fs.FS
	VFS *VFS
}

func NewFS(config Config, logger zLogger.ZLogger) (VFSRW, error) {

	logger = new(logger.With().Str("module", "vfsrw").Logger())

	vfs := &vFSRW{
		fss:    map[string]vfsStruct{},
		logger: logger,
	}

	if err := vfs.init(config); err != nil {
		return nil, errors.Wrap(err, "cannot initialize vfsrw")
	}
	return vfs, nil
}

func NewFSWithMiniResolver(config Config, miniResolverClient *resolver.MiniResolver, logger zLogger.ZLogger) (*vFSRW, error) {
	logger = new(logger.With().Str("module", "vfsrw").Logger())

	vfs := &vFSRW{
		fss:                map[string]vfsStruct{},
		logger:             logger,
		miniResolverClient: miniResolverClient,
	}

	if err := vfs.init(config); err != nil {
		return nil, errors.Wrap(err, "cannot initialize vfsrw")
	}
	return vfs, nil
}

func (vfs *vFSRW) init(config Config) error {
	var toClose = []io.Closer{}
	var closeAll = func() {
		// iterate in reverse order
		last := len(toClose) - 1
		for i := range toClose {
			c := toClose[last-i]
			c.Close()
		}
	}

	for pass := 0; pass < 2; pass++ {
		for name, cfg := range config {
			if _, ok := vfs.fss[name]; ok {
				continue
			}
			logger := new(vfs.logger.With().Str("fs", name).Logger())
			switch strings.ToLower(cfg.Type) {
			case "minikvstore":
				if cfg.MiniKVStore == nil {
					closeAll()
					return errors.Errorf("no minikvstore section for filesystem '%s'", cfg.Name)
				}
				xFS, closers, err := vfs.newMiniKVStore(name, cfg.MiniKVStore, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create minikvstore in '%s'", cfg.Name)
				}
				toClose = append(toClose, closers...)
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			case "web":
				if cfg.Web == nil {
					closeAll()
					return errors.Errorf("no web section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newWeb(name, cfg.Web, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create webfs in '%s'", cfg.Name)
				}
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			case "os":
				if cfg.OS == nil {
					closeAll()
					return errors.Errorf("no os section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newOS(name, cfg.OS, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create osfs in '%s'", cfg.Name)
				}
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			case "sftp":
				if cfg.SFTP == nil {
					closeAll()
					return errors.Errorf("no sftp section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newSFTP(name, cfg.SFTP, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create sftpfsrw in '%s'", cfg.Name)
				}
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			case "s3":
				if cfg.S3 == nil {
					closeAll()
					return errors.Errorf("no s3 section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newS3(name, cfg.S3, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create s3fsrw in '%s'", cfg.Name)
				}
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			case "remote":
				if cfg.Remote == nil {
					closeAll()
					return errors.Errorf("no Remote section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newRemote(name, cfg.Remote, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create s3fsrw in '%s'", cfg.Name)
				}
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			case "afero":
				if cfg.Afero == nil {
					closeAll()
					return errors.Errorf("no afero section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newAfero(name, cfg.Afero, cfg.ReadOnly, logger)
				if err != nil {
					if pass == 0 {
						continue
					}
					closeAll()
					return errors.Wrapf(err, "cannot create aferofs in '%s'", cfg.Name)
				}
				if cfg.ZipAsFolder != nil && cfg.ZipAsFolder.Enabled && cfg.ZipAsFolder.CacheSize > 0 {
					zFS, err := zipasfolder.NewFS(xFS, int(cfg.ZipAsFolder.CacheSize), time.Duration(cfg.ZipAsFolder.Timeout), cfg.ReadOnly || cfg.ZipAsFolder.ReadOnly, logger)
					if err != nil {
						closeAll()
						return errors.Wrapf(err, "cannot create zipasfolder over '%v'", xFS)
					}
					xFS = zFS
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = vfsStruct{FS: xFS, VFS: cfg}
			}
		}
	}
	return nil
}

type vFSRW struct {
	fss                      map[string]vfsStruct
	miniResolverClient       *resolver.MiniResolver
	miniResolverClientTLS    *tls.Config
	miniResolverClientLoader loader.Loader
	logger                   zLogger.ZLogger
}

func (vfs *vFSRW) IsEmpty(dir string) (bool, error) {
	vFS, pathStr, err := vfs.getFS(dir)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return writefs.IsEmpty(vFS, pathStr)
}

func (vfs *vFSRW) RealPath(pathStr string) string {
	pathStr, name, newPath, err := MatchPath(pathStr)
	if err != nil {
		vfs.logger.Error().Err(err).Msgf("cannot match path '%s'", pathStr)
		return pathStr
	}
	return fmt.Sprintf("vfs:/%s/%s", name, newPath)
}

func (vfs *vFSRW) Sub(dir string) (fs.FS, error) {
	fSys, pathStr, err := vfs.getFS(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get FS for path '%s'", dir)
	}
	if subFS, ok := fSys.(writefs.SubFS); ok {
		return subFS.Sub(pathStr)
	}
	/*
		if strings.HasSuffix(dir, ".zip") {
			conf, err := vfs.getConfig(dir)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot get config for zip file '%s'", dir)
			}
			if conf != nil && conf.ZipAsFolder != nil && conf.ZipAsFolder.Enabled {
				if _, err := fs.Stat(vfs, dir); err != nil {
					return nil, errors.Wrapf(err, "cannot stat zip file '%s'", pathStr)
				}
				return zipfsw.NewFSFile(fSys, pathStr, false, vfs.logger)
			}
		}
	*/

	return writefs.NewSubFS(vfs, dir)
}

func (vfs *vFSRW) SubCreate(dir string) (fs.FS, error) {
	conf, err := vfs.getConfig(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	zipAsFolder := conf != nil && conf.ZipAsFolder != nil && conf.ZipAsFolder.Enabled && strings.HasSuffix(dir, ".zip")

	if !zipAsFolder {
		if err := writefs.MkDir(vfs, dir); err != nil {
			if !errors.Is(err, writefs.ErrNotImplemented) {
				return nil, errors.WithStack(err)
			}
		}
		return writefs.NewSubFS(vfs, dir)
	}
	fi, err := fs.Stat(vfs, dir)
	if err == nil {
		if !fi.IsDir() {
			return nil, errors.Errorf("cannot use sub on file '%s'", dir)
		}
		vFS, pathStr, err := vfs.getFS(dir)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return zipfsw.NewFSFile(vFS, pathStr, true, vfs.logger)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, errors.WithStack(err)
	}
	if conf.ReadOnly || (conf.ZipAsFolder != nil && conf.ZipAsFolder.ReadOnly) {
		return nil, errors.Wrapf(fs.ErrNotExist, "cannot create zip file '%s' in read-only filesystem", dir)
	}
	fp, err := writefs.Create(vfs, dir)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zip file '%s'", dir)
	}
	var encFP io.WriteCloser
	if conf.ZipAsFolder != nil && conf.ZipAsFolder.AES != nil && conf.ZipAsFolder.AES.Enable {
		_ = fp.Close()
		_ = encFP
		return nil, errors.Errorf("cannot create encrypted zip file '%s' not implemented", dir)
		//todo: implement encryption
		/*
			aesConfig := conf.ZipAsFolder.AES
			encFileName := fmt.Sprintf("%s.aes", dir)
			encFP, err = writefs.Create(vfs, encFileName)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot create encrypted zip file '%s.aes'", dir)
			}
			db, err := keepass2kms.LoadKeePassDBFromFile(aesConfig.KeepassFile.String(), aesConfig.KeepassKey.String())
			if err != nil {
				return nil, errors.Wrapf(err, "cannot load keepass file '%s'", aesConfig.KeepassFile)
			}
			client, err := keepass2kms.NewClient(db, filepath.Base(aesConfig.KeepassFile.String()))
			if err != nil {
				return nil, errors.Wrap(err, "cannot create keepass2kms client")
			}
			registry.RegisterKMSClient(client)
			// todo: check for existence of key

			// add a buffer to the file
			newEncFPBuffer := bufio.NewWriterSize(encFP, 1024*1024)

			csEncWriter, err := checksum.NewChecksumWriter(conf.ZipAsFolder.Digests, newEncFPBuffer)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot create checksum writer for '%s'", encFileName)
			}

			encWriter, err := encrypt.NewEncryptWriterAESGCM(csEncWriter, []byte(path), nil)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot create encrypt writer for '%s'", encFileName)
			}

			handle := encWriter.GetKeysetHandle()

			if err := fsFactory.Register(zipfsrw.NewCreateFSEncryptedChecksumFunc(noCompression, zipDigests, string(aesConfig.KeepassEntry), logger.Logger()), "\\.zip$", writefs.HighFS); err != nil {
				return nil, errors.Wrap(err, "cannot register FSEncryptedChecksum")
			}
		*/

	}
	zipFS, err := zipfsw.NewFS(
		fp,
		true,
		!conf.ZipAsFolder.Compress,
		dir,
		conf.ZipAsFolder.Digests,
		func(css map[checksum.DigestAlgorithm]string) error {
			for digestAlg, digest := range css {
				digestFile := fmt.Sprintf("%s.%s", dir, digestAlg)
				if _, err := writefs.WriteFile(vfs, digestFile, []byte(fmt.Sprintf("%s *%s", digest, filepath.Base(dir)))); err != nil {
					return errors.Wrapf(err, "failed to write digest file '%s'", digestFile)
				}
			}
			return nil
		},
		vfs.logger,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return zipFS, nil
}

func (vfs *vFSRW) IsWriteable(name string) bool {
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		return false
	}
	return writefs.IsWriteable(vFS, pathStr)
}

func (vfs *vFSRW) Get(name string, readOnly bool) (fs.FS, error) {
	u, err := url.Parse(name)
	if err == nil && u.Scheme == "vfs" {
		name = u.Host
	} else if strings.HasPrefix(name, "vfs://") {
		name = strings.TrimPrefix(name, "vfs://")
		name = strings.TrimSuffix(name, "/")
	}

	f, ok := vfs.fss[name]
	if !ok {
		return nil, errors.Errorf("filesystem %s not found", name)
	}
	return f.FS, nil
}

func (vfs *vFSRW) AddFS(name string, vfsConfig *VFS, fsys fs.FS) {
	if vfsConfig == nil {
		vfsConfig = &VFS{
			ReadOnly: true,
			Name:     name,
		}

	}
	var xFS fs.FS = fsys
	if vfsConfig.ZipAsFolder != nil && vfsConfig.ZipAsFolder.Enabled && vfsConfig.ZipAsFolder.CacheSize > 0 {
		zFS, err := zipasfolder.NewFS(
			fsys,
			int(vfsConfig.ZipAsFolder.CacheSize),
			time.Duration(vfsConfig.ZipAsFolder.Timeout),
			vfsConfig.ReadOnly || vfsConfig.ZipAsFolder.ReadOnly,
			vfs.logger,
		)
		if err != nil {
			vfs.logger.Error().Err(err).Msgf("cannot create zipasfolder over '%v'", fsys)
		} else {
			xFS = zFS
		}
	}

	vfs.fss[name] = vfsStruct{FS: xFS, VFS: vfsConfig}
}

func (vfs *vFSRW) Equal(fsys fs.FS) bool {
	if vFS, ok := fsys.(*vFSRW); ok {
		if len(vFS.fss) != len(vfs.fss) {
			return false
		}
		for name, fsStruct := range vfs.fss {
			fs2Struct, ok := vFS.fss[name]
			if !ok {
				return false
			}
			if !writefs.Equal(fsStruct.FS, fs2Struct.FS) {
				return false
			}
		}
		return true
	}
	return false
}

func (vfs *vFSRW) Close() error {
	var errs = []error{}
	if vfs.miniResolverClient != nil {
		if err := vfs.miniResolverClient.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "cannot close miniresolver client"))
		}
	}
	if vfs.miniResolverClientLoader != nil {
		if err := vfs.miniResolverClientLoader.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "cannot close miniresolver loader"))
		}
	}
	for _, fss := range vfs.fss {
		if closer, ok := fss.FS.(io.Closer); ok {
			err := closer.Close()
			if err != nil {
				errs = append(errs, errors.WithStack(err))
			}
		}
	}
	return errors.Combine(errs...)
}

func (vfs *vFSRW) Remove(name string) error {
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		return errors.WithStack(err)
	}
	err = writefs.Remove(vFS, pathStr)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (vfs *vFSRW) Rename(oldPath, newPath string) error {
	oldPath, name1, _, _ := MatchPath(oldPath)
	newPath, name2, _, _ := MatchPath(newPath)
	if name2 != name1 {
		return errors.Errorf("cannot rename over multiple filesystems %s -> %s", name1, name2)
	}

	vFS, op, err := vfs.getFS(oldPath)
	if err != nil {
		return errors.WithStack(err)
	}
	_, np, err := vfs.getFS(newPath)
	if err != nil {
		return errors.WithStack(err)
	}
	err = writefs.Rename(vFS, op, np)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil

}

func (vfs *vFSRW) MkDir(name string) error {
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		return errors.WithStack(err)
	}
	err = writefs.MkDir(vFS, pathStr)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (vfs *vFSRW) Create(name string) (writefs.FileWrite, error) {
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := writefs.Create(vFS, pathStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

func (vfs *vFSRW) String() string {
	names := []string{}
	for name, _ := range vfs.fss {
		names = append(names, name)
	}
	return fmt.Sprintf("vFSRW(%v)", names)
}

func (vfs *vFSRW) Stat(name string) (fs.FileInfo, error) {
	vfs.logger.Debug().Msgf("VFS.Stat('%s')", name)
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		vfs.logger.Debug().Msgf("VFS.Stat('%s') -> getFS error: %v", name, err)
		return nil, errors.WithStack(err)
	}
	vfs.logger.Debug().Msgf("VFS.Stat('%s') -> using FS for path '%s'", name, pathStr)
	data, err := fs.Stat(vFS, pathStr)
	if err != nil {
		vfs.logger.Debug().Msgf("VFS.Stat('%s') -> fs.Stat('%s') error: %v", name, pathStr, err)
		return nil, errors.WithStack(err)
	}
	return data, nil
}

func (vfs *vFSRW) ReadFile(name string) ([]byte, error) {
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := fs.ReadFile(vFS, pathStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

type subDirEntry struct {
	fs.DirEntry
	name string
}

func (s subDirEntry) Name() string {
	return s.name
}

var _ fs.DirEntry = &subDirEntry{}

func (vfs *vFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	vFS, pathStr, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	de, err := fs.ReadDir(vFS, pathStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	newEntries := []fs.DirEntry{}
	for _, d := range de {
		newEntries = append(newEntries, &subDirEntry{d, path.Join(strings.Replace(name, "//", "/", 1), d.Name())})
	}
	return newEntries, nil
}

func (vfs *vFSRW) Open(vfsPath string) (fs.File, error) {
	vFS, pathStr, err := vfs.getFS(vfsPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fp, err := vFS.Open(pathStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return fp, nil
}

func (vfs *vFSRW) getFS(vfsPath string) (fs.FS, string, error) {
	vfsPath, name, pathStr, err := MatchPath(vfsPath)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}
	vfsStruct, ok := vfs.fss[name]
	if !ok {
		return nil, "", errors.Errorf("vfs '%s' not configured for path '%s'", name, vfsPath)
	}
	return vfsStruct.FS, pathStr, nil
}
func (vfs *vFSRW) getConfig(vfsPath string) (*VFS, error) {
	vfsPath, name, _, err := MatchPath(vfsPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	vStruct, ok := vfs.fss[name]
	if !ok {
		return nil, errors.Errorf("vfs '%s' not configured for path '%s'", name, vfsPath)
	}
	return vStruct.VFS, nil
}

func (vfs *vFSRW) Join(fsys fs.FS, elems ...string) string {
	if !strings.HasPrefix(elems[0], "vfs:/") {
		return filepath.ToSlash(filepath.Join(elems...))
	}
	newElems := make([]string, len(elems))
	newElems[0] = filepath.ToSlash(strings.TrimPrefix(elems[0][5:], "/"))
	for i, elem := range elems[1:] {
		newElems[i+1] = filepath.ToSlash(elem)
	}
	return "vfs://" + path.Join(newElems...)
}

func (vfs *vFSRW) Copy(src, dst string) (int64, error) {
	src, srcName, srcPath, err := MatchPath(src)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	srcFSStruct, ok := vfs.fss[srcName]
	if !ok {
		return 0, errors.Errorf("vfs '%s' not configured for path '%s'", srcName, src)
	}
	srcFS := srcFSStruct.FS
	dst, dstName, dstPath, err := MatchPath(dst)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	dstFSStruct, ok := vfs.fss[dstName]
	if !ok {
		return 0, errors.Errorf("vfs '%s' not configured for path '%s'", dstName, dst)
	}
	dstFS := dstFSStruct.FS
	if writefs.Equal(srcFS, dstFS) {
		if copyFS, ok := srcFS.(writefs.CopyFS); ok {
			num, err := copyFS.Copy(srcPath, dstPath)
			if err == nil {
				return num, nil
			}
			return num, errors.Wrapf(err, "cannot copy '%s' -> '%s' on '%s'", src, dst, srcName)
		}
	}
	srcFP, err := srcFS.Open(srcPath)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot open source '%s'", src)
	}
	defer srcFP.Close()
	dstFP, err := writefs.Create(dstFS, dstPath)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot create destination '%s'", dst)
	}
	num, err := io.Copy(dstFP, srcFP)
	if err != nil {
		dstFP.Close()
		return 0, errors.Wrapf(err, "cannot copy '%s' -> '%s'", src, dst)
	}
	if err := dstFP.Close(); err != nil {
		return 0, errors.Wrapf(err, "cannot close destination '%s'", dst)
	}
	return num, nil
}

var (
	_ VFSRW = (*vFSRW)(nil)
)
