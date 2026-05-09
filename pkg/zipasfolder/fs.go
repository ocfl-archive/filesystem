package zipasfolder

import (
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/bluele/gcache"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
)

// NewFS creates a new zipAsFolderFS which handles zipfiles like folders which are read-only
// it implements readwritefs.ReadWriteFS, fs.ReadDirFS, fs.ReadFileFS, basefs.CloserFS
func NewFS(baseFS fs.FS, cacheSize int, readOnly bool, logger zLogger.ZLogger) (*zipAsFolderFS, error) {
	_logger := logger.With().Str("class", "zipAsFolderFS").Logger()
	logger = &_logger
	f := &zipAsFolderFS{
		baseFS:   baseFS,
		readOnly: readOnly,
		zipCache: gcache.New(cacheSize).
			LRU().
			LoaderFunc(func(key interface{}) (interface{}, error) {
				zipFilename, ok := key.(string)
				logger.Debug().Msgf("load zip file '%s'", zipFilename)
				if !ok {
					return nil, errors.Errorf("cannot cast key %v to string", key)
				}
				zipFile, err := baseFS.Open(zipFilename)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot open zip file '%s'", zipFilename)
				}
				zipFS, err := NewZipFSCloser(zipFile, zipFilename, logger)
				if err != nil {
					logger.Debug().Msgf("load zip file '%s' FAILED: %v", zipFilename, err)
					return nil, errors.Wrapf(err, "cannot create zipfscloser for '%s'", zipFilename)
				}
				return zipFS, nil
			}).
			EvictedFunc(func(key, value any) {
				logger.Debug().Msgf("evict zip file '%s'", key)
				zipFS, ok := value.(fs.FS)
				if !ok {
					return
				}
				// wait if it is locked or has open files
				for range 100 {
					locked := false
					if lockedFS, ok := zipFS.(writefs.IsLockedFS); ok {
						if lockedFS.IsLocked() {
							locked = true
						}
					}
					if refFS, ok := zipFS.(IsRefCountFS); ok {
						if refFS.RefCount() > 0 {
							locked = true
						}
					}
					if !locked {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				writefs.Close(zipFS)
			}).
			PurgeVisitorFunc(func(key, value any) {
				logger.Debug().Msgf("purge zip file '%s'", key)
				zipFS, ok := value.(fs.FS)
				if !ok {
					return
				}
				// wait if it is locked or has open files
				for range 100 {
					locked := false
					if lockedFS, ok := zipFS.(writefs.IsLockedFS); ok {
						if lockedFS.IsLocked() {
							locked = true
						}
					}
					if refFS, ok := zipFS.(IsRefCountFS); ok {
						if refFS.RefCount() > 0 {
							locked = true
						}
					}
					if !locked {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				writefs.Close(zipFS)
			}).
			Build(),
		end:    make(chan bool),
		logger: logger,
	}
	go func() {
		for alive := true; alive; {
			timer := time.NewTimer(time.Minute)
			select {
			case <-f.end:
				timer.Stop()
				alive = false
			case <-timer.C:
				f.ClearUnlocked()
			}
		}
	}()
	return f, nil
}

type zipAsFolderFS struct {
	baseFS   fs.FS
	zipCache gcache.Cache
	lock     sync.RWMutex
	end      chan bool
	logger   zLogger.ZLogger
	readOnly bool
}

func (fsys *zipAsFolderFS) Equal(fsys2 fs.FS) bool {
	if fsys2, ok := fsys2.(*zipAsFolderFS); ok {
		return fsys.baseFS == fsys2.baseFS
	}
	return false
}

func (fsys *zipAsFolderFS) Fullpath(name string) (string, error) {
	return writefs.Fullpath(fsys.baseFS, name)
}

func (fsys *zipAsFolderFS) String() string {
	return "zipAsFolderFS"
}

// CReate creates a new file
func (fsys *zipAsFolderFS) Create(path string) (writefs.FileWrite, error) {
	if fsys.readOnly {
		return nil, errors.New("read-only filesystem")
	}
	path = clearPath(path)
	zipFile, zipPath, isZIP := expandZipFile(path)
	if isZIP && zipPath != "" {
		return nil, errors.Errorf("cannot create file '%s' in zip file '%s'", path, zipFile)
	}
	return writefs.Create(fsys.baseFS, path)
}

// MkDir creates a new folder
func (fsys *zipAsFolderFS) MkDir(path string) error {
	if fsys.readOnly {
		return errors.New("read-only filesystem")
	}
	path = clearPath(path)
	zipFile, zipPath, isZIP := expandZipFile(path)
	if isZIP && zipPath != "" {
		return errors.Errorf("cannot create folder '%s' in zip file '%s'", path, zipFile)
	}
	return writefs.MkDir(fsys.baseFS, path)
}

// Stat returns the file info for a given path
func (fsys *zipAsFolderFS) Stat(name string) (fs.FileInfo, error) {
	fsys.logger.Debug().Msgf("Stat('%s')", name)
	name = clearPath(name)
	zipFile, zipPath, isZIP := expandZipFile(name)
	fsys.logger.Debug().Msgf("expandZipFile('%s') -> zipFile='%s', zipPath='%s', isZIP=%v", name, zipFile, zipPath, isZIP)
	if !isZIP {
		if name == "" {
			name = "."
		}
		info, err := fs.Stat(fsys.baseFS, name)
		if err != nil {
			return info, errors.Wrapf(err, "cannot stat file '%s'", name)
		}
		return info, nil
	}
	if zipPath == "" || zipPath == "." {
		// handle the zip file itself as a directory
		fsys.lock.Lock()
		zipFSCache, err := fsys.zipCache.Get(zipFile)
		if err != nil {
			fsys.lock.Unlock()
			fsys.logger.Debug().Msgf("Stat: cannot get zip file '%s' from cache: %v", zipFile, err)
			// check if it is just a file
			info, err2 := fs.Stat(fsys.baseFS, zipFile)
			if err2 == nil {
				fsys.logger.Debug().Msgf("Stat: zip file '%s' exists on baseFS (type %T), returning its Info (IsDir=%v)", zipFile, fsys.baseFS, info.IsDir())
				// If it's already a directory, return it as is
				if info.IsDir() {
					return info, nil
				}
				// Return a wrapped FileInfo that says it's a directory
				return writefs.NewFileInfoDir(info.Name()), nil
			}
			fsys.logger.Debug().Msgf("Stat: zip file '%s' NOT found on baseFS (baseFS type: %T): %v", zipFile, fsys.baseFS, err2)
			return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
		}
		fsys.lock.Unlock()
		if statFS, ok := zipFSCache.(fs.StatFS); ok {
			_, err := statFS.Stat(".")
			if err == nil {
				// ensure the name is the zip file name, not "."
				return writefs.NewFileInfoDir(filepath.Base(zipFile)), nil
			}
		}
		return writefs.NewFileInfoDir(filepath.Base(zipFile)), nil
	}
	fsys.lock.Lock()
	zipFSCache, err := fsys.zipCache.Get(zipFile)
	if err != nil {
		fsys.lock.Unlock()
		return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
	}

	zipFS, ok := zipFSCache.(fs.FS)
	if !ok {
		fsys.lock.Unlock()
		return nil, errors.Errorf("cannot cast zip file '%s' type %T to fs.FS", zipFile, zipFSCache)
	}
	defer fsys.lock.Unlock()
	return fs.Stat(zipFS, zipPath)
}

// ReadFile reads a file from the filesystem
func (fsys *zipAsFolderFS) ReadFile(name string) ([]byte, error) {
	fp, err := fsys.Open(name)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s'", name)
	}
	defer fp.Close()
	return io.ReadAll(fp)
}

// ReadDir reads a directory from the filesystem
func (fsys *zipAsFolderFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fsys.logger.Debug().Msgf("ReadDir('%s')", name)
	name = clearPath(name)
	zipFile, zipPath, isZIP := expandZipFile(name)
	fsys.logger.Debug().Msgf("expandZipFile('%s') -> zipFile='%s', zipPath='%s', isZIP=%v", name, zipFile, zipPath, isZIP)
	if !isZIP {
		if name == "" {
			name = "."
		}
		entries, err := fs.ReadDir(fsys.baseFS, name)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot read directory '%s'", name)
		}
		var result = make([]fs.DirEntry, 0, len(entries))
		for _, entry := range entries {
			if isZipFile(entry.Name()) {
				result = append(result, writefs.NewDirEntry(writefs.NewFileInfoDir(entry.Name())))
			} else {
				result = append(result, entry)
			}
		}
		return result, nil
	}
	if zipPath == "" || zipPath == "." {
		zipPath = "."
	}
	fsys.lock.Lock()
	zipFSCache, err := fsys.zipCache.Get(zipFile)
	if err != nil {
		fsys.lock.Unlock()
		return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
	}
	zipFS, ok := zipFSCache.(fs.FS)
	if !ok {
		fsys.lock.Unlock()
		return nil, errors.Errorf("cannot cast zip file '%s' to fs.FS", zipFile)
	}
	if zipPath == "" {
		zipPath = "."
	}
	defer fsys.lock.Unlock()
	entries, err := fs.ReadDir(zipFS, zipPath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read directory '%s' in zip '%s'", zipPath, zipFile)
	}
	return entries, nil
}

// Open opens a file from the filesystem
func (fsys *zipAsFolderFS) Open(name string) (fs.File, error) {
	fsys.logger.Debug().Msgf("Open('%s')", name)
	name = clearPath(name)
	zipFile, zipPath, isZIP := expandZipFile(name)
	fsys.logger.Debug().Msgf("expandZipFile('%s') -> zipFile='%s', zipPath='%s', isZIP=%v", name, zipFile, zipPath, isZIP)
	if !isZIP {
		file, err := fsys.baseFS.Open(name)
		if err != nil {
			return file, errors.Wrapf(err, "cannot open file '%s'", name)
		}
		return file, nil
	}

	fsys.lock.Lock()
	zipFSCache, err := fsys.zipCache.Get(zipFile)
	if err != nil {
		fsys.lock.Unlock()
		return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
	}
	zipFS, ok := zipFSCache.(fs.FS)
	if !ok {
		fsys.lock.Unlock()
		return nil, errors.Errorf("cannot cast zip file '%s' to fs.FS", zipFile)
	}
	defer fsys.lock.Unlock()
	rc, err := zipFS.Open(zipPath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s' in zip file '%s'", zipPath, zipFile)
	}
	return rc, nil
}

// Close closes the filesystem and underlying fs if possible
func (fsys *zipAsFolderFS) Close() error {
	fsys.lock.Lock()
	defer fsys.lock.Unlock()
	fsys.end <- true
	fsys.zipCache.Purge()

	if closer, ok := fsys.baseFS.(io.Closer); ok {
		closer.Close()
	}
	return nil
}

func (fsys *zipAsFolderFS) ClearUnlocked() error {
	fsys.lock.Lock()
	defer fsys.lock.Unlock()
	fsMap := fsys.zipCache.GetALL(false)
	for key, mFS := range fsMap {
		isLockedFS, ok := mFS.(writefs.IsLockedFS)
		if !ok {
			// fallback if it doesn't implement IsLockedFS, but zipfs should
			continue
		}
		if !isLockedFS.IsLocked() {
			fsys.zipCache.Remove(key)
		}
	}
	return nil
}
func isZipFile(name string) bool {
	return strings.ToLower(filepath.Ext(name)) == ".zip"
}

func expandZipFile(name string) (zipFile string, zipPath string, isZip bool) {
	name = filepath.ToSlash(filepath.Clean(name))
	if name == "." {
		return ".", "", false
	}
	parts := strings.Split(name, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if isZipFile(parts[i]) {
			zipFile = strings.Join(parts[:i+1], "/")
			zipPath = strings.Join(parts[i+1:], "/")
			isZip = true
			return
		}
	}
	return name, "", false
}

var (
	_ writefs.CreateFS   = (*zipAsFolderFS)(nil)
	_ writefs.MkDirFS    = (*zipAsFolderFS)(nil)
	_ writefs.CloseFS    = (*zipAsFolderFS)(nil)
	_ writefs.FullpathFS = (*zipAsFolderFS)(nil)
	_ writefs.EqualFS    = (*zipAsFolderFS)(nil)
	_ fs.FS              = (*zipAsFolderFS)(nil)
	_ fs.ReadDirFS       = (*zipAsFolderFS)(nil)
	_ fs.ReadFileFS      = (*zipAsFolderFS)(nil)
	_ fs.StatFS          = (*zipAsFolderFS)(nil)
)
