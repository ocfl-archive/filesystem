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

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
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
	fs.FS
	fs.ReadDirFS
	fs.ReadFileFS
	fs.StatFS
}

func NewFS(config Config, logger zLogger.ZLogger) (VFSRW, error) {

	_logger := logger.With().Str("module", "vfsrw").Logger()
	logger = &_logger

	vfs := &vFSRW{
		fss:    map[string]fs.FS{},
		logger: logger,
	}

	if err := vfs.init(config); err != nil {
		return nil, errors.Wrap(err, "cannot initialize vfsrw")
	}
	return vfs, nil
}

func NewFSWithMiniResolver(config Config, miniResolverClient *resolver.MiniResolver, logger zLogger.ZLogger) (*vFSRW, error) {
	_logger := logger.With().Str("module", "vfsrw").Logger()
	logger = &_logger

	vfs := &vFSRW{
		fss:                map[string]fs.FS{},
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
			_logger := vfs.logger.With().Str("fs", name).Logger()
			logger := &_logger
			switch strings.ToLower(cfg.Type) {
			case "minikvstore":
				if cfg.MiniKVStore == nil {
					closeAll()
					return errors.Errorf("no minikvstore section for filesystem '%s'", cfg.Name)
				}
				mkvsFS, closers, err := vfs.newMiniKVStore(name, cfg.MiniKVStore, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create minikvstore in '%s'", cfg.Name)
				}
				toClose = append(toClose, closers...)
				if closer, ok := mkvsFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = mkvsFS
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
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
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
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
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
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
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
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
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
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
			case "memfs":
				if cfg.MemFS == nil {
					closeAll()
					return errors.Errorf("no memfs section for filesystem '%s'", cfg.Name)
				}
				xFS, err := vfs.newMemFS(name, cfg.MemFS, cfg.ReadOnly, logger)
				if err != nil {
					closeAll()
					return errors.Wrapf(err, "cannot create memfs in '%s'", cfg.Name)
				}
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
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
				if closer, ok := xFS.(io.Closer); ok {
					toClose = append(toClose, closer)
				}
				vfs.fss[cfg.Name] = xFS
			}
		}
	}
	return nil
}

type vFSRW struct {
	fss                      map[string]fs.FS
	miniResolverClient       *resolver.MiniResolver
	miniResolverClientTLS    *tls.Config
	miniResolverClientLoader loader.Loader
	logger                   zLogger.ZLogger
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
	return f, nil
}

func (vfs *vFSRW) AddFS(name string, fsys fs.FS) {
	vfs.fss[name] = fsys
}

func (vfs *vFSRW) Equal(fsys fs.FS) bool {
	if vFS, ok := fsys.(*vFSRW); ok {
		if len(vFS.fss) != len(vfs.fss) {
			return false
		}
		for name, fs := range vfs.fss {
			fs2, ok := vFS.fss[name]
			if !ok {
				return false
			}
			if !writefs.Equal(fs, fs2) {
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
	for _, fs := range vfs.fss {
		if closer, ok := fs.(io.Closer); ok {
			err := closer.Close()
			if err != nil {
				errs = append(errs, errors.WithStack(err))
			}
		}
	}
	return errors.Combine(errs...)
}

func (vfs *vFSRW) Remove(name string) error {
	vFS, path, err := vfs.getFS(name)
	if err != nil {
		return errors.WithStack(err)
	}
	err = writefs.Remove(vFS, path)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (vfs *vFSRW) Rename(oldPath, newPath string) error {
	name1, _, _ := matchPath(oldPath)
	name2, _, _ := matchPath(newPath)
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
	vFS, path, err := vfs.getFS(name)
	if err != nil {
		return errors.WithStack(err)
	}
	err = writefs.MkDir(vFS, path)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (vfs *vFSRW) Create(name string) (writefs.FileWrite, error) {
	vFS, path, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := writefs.Create(vFS, path)
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
	vFS, path, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := fs.Stat(vFS, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

func (vfs *vFSRW) ReadFile(name string) ([]byte, error) {
	vFS, path, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := fs.ReadFile(vFS, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

func (vfs *vFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	vFS, path, err := vfs.getFS(name)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	de, err := fs.ReadDir(vFS, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return de, nil
}

func (vfs *vFSRW) Open(vfsPath string) (fs.File, error) {
	vFS, path, err := vfs.getFS(vfsPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fp, err := vFS.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return fp, nil
}

func (vfs *vFSRW) getFS(vfsPath string) (fs.FS, string, error) {
	name, path, err := matchPath(vfsPath)
	if err != nil {
		return nil, "", errors.WithStack(err)
	}
	vFS, ok := vfs.fss[name]
	if !ok {
		return nil, "", errors.Errorf("vfs '%s' not configured for path '%s'", name, vfsPath)
	}
	return vFS, path, nil
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
	srcName, srcPath, err := matchPath(src)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	srcFS, ok := vfs.fss[srcName]
	if !ok {
		return 0, errors.Errorf("vfs '%s' not configured for path '%s'", srcName, src)
	}
	dstName, dstPath, err := matchPath(dst)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	dstFS, ok := vfs.fss[dstName]
	if !ok {
		return 0, errors.Errorf("vfs '%s' not configured for path '%s'", dstName, dst)
	}
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
