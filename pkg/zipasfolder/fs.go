package zipasfolder

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v4/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/maypok86/otter/v2"
)

type fileCloser struct {
	fs.File
	closeFunc func() error
}

func (fc *fileCloser) Close() error {
	err := fc.File.Close()
	if fc.closeFunc != nil {
		if err2 := fc.closeFunc(); err2 != nil {
			if err == nil {
				err = err2
			}
		}
	}
	return err
}

// NewFS creates a new zipAsFolderFS which handles zipfiles like folders which are read-only
// it implements readwritefs.ReadWriteFS, fs.ReadDirFS, fs.ReadFileFS, basefs.CloserFS
func NewFS(baseFS fs.FS, cacheSize int, readOnly bool, logger zLogger.ZLogger) (*zipAsFolderFS, error) {
	logger = new(logger.With().Str("class", "zipAsFolderFS").Logger())
	f := &zipAsFolderFS{
		baseFS:   baseFS,
		readOnly: readOnly,
		end:      make(chan bool),
		logger:   logger,
		toClose:  []fs.FS{},
	}
	zipCache, err := otter.New[string, fs.FS](&otter.Options[string, fs.FS]{
		MaximumSize: cacheSize,
		OnAtomicDeletion: func(e otter.DeletionEvent[string, fs.FS]) {
			zipFS := e.Value

			f.lock.Lock()
			rc := getRefCount(zipFS)
			if isLocked(zipFS) || rc > 0 {
				f.toClose = append(f.toClose, zipFS)
				f.lock.Unlock()
			} else {
				f.lock.Unlock()
				writefs.Close(zipFS)
			}
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "cannot create otter cache")
	}
	f.zipCache = zipCache
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
	zipCache *otter.Cache[string, fs.FS]
	lock     sync.RWMutex
	end      chan bool
	logger   zLogger.ZLogger
	readOnly bool
	toClose  []fs.FS
}

func (fsys *zipAsFolderFS) IsWriteable(path string) bool {
	if fsys.readOnly {
		return false
	}
	zipFile, zipPath, isZIP := expandZipFile(clearPath(path))
	if isZIP && zipPath != "" {
		return false
	}
	return writefs.IsWriteable(fsys.baseFS, zipFile)
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
	return fmt.Sprintf("zipAsFolderFS(%s)", fsys.baseFS)
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

func (fsys *zipAsFolderFS) getZipFS(zipFile string) (fs.FS, error) {
	fsys.lock.Lock()
	defer fsys.lock.Unlock()

	if zipFS, ok := fsys.zipCache.GetIfPresent(zipFile); ok {
		if !isClosed(zipFS) {
			incRef(zipFS)
			return zipFS, nil
		}
	}

	fsys.logger.Debug().Msgf("load zip file '%s'", zipFile)
	f, err := fsys.baseFS.Open(zipFile)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open zip file '%s'", zipFile)
	}
	zipFS, err := NewZipFSCloser(f, zipFile, fsys.logger)
	if err != nil {
		fsys.logger.Debug().Msgf("load zip file '%s' FAILED: %v", zipFile, err)
		return nil, errors.Wrapf(err, "cannot create zipfscloser for '%s'", zipFile)
	}
	incRef(zipFS)
	fsys.zipCache.Set(zipFile, zipFS)
	// Ensure otter processes the addition if possible,
	// though Set is usually enough to make it visible to GetIfPresent
	return zipFS, nil
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
		zipFS, err := fsys.getZipFS(zipFile)
		if err != nil {
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
		defer decRef(zipFS)
		if statFS, ok := zipFS.(fs.StatFS); ok {
			_, err := statFS.Stat(".")
			if err == nil {
				// ensure the name is the zip file name, not "."
				return writefs.NewFileInfoDir(filepath.Base(zipFile)), nil
			}
		}
		return writefs.NewFileInfoDir(filepath.Base(zipFile)), nil
	}
	zipFS, err := fsys.getZipFS(zipFile)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
	}

	defer decRef(zipFS)

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
	zipFS, err := fsys.getZipFS(zipFile)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
	}

	defer decRef(zipFS)

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

	zipFS, err := fsys.getZipFS(zipFile)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get zip file '%s'", zipFile)
	}

	rc, err := zipFS.Open(zipPath)
	if err != nil {
		decRef(zipFS)
		return nil, errors.Wrapf(err, "cannot open file '%s' in zip file '%s'", zipPath, zipFile)
	}
	return &fileCloser{
		File: rc,
		closeFunc: func() error {
			decRef(zipFS)
			return nil
		},
	}, nil
}

// Close closes the filesystem and underlying fs if possible
func (fsys *zipAsFolderFS) Close() error {
	fsys.end <- true
	fsys.zipCache.InvalidateAll()

	if closer, ok := fsys.baseFS.(io.Closer); ok {
		closer.Close()
	}
	return nil
}

func (fsys *zipAsFolderFS) ClearUnlocked() error {
	for key, value := range fsys.zipCache.All() {
		if !isLocked(value) && getRefCount(value) == 0 {
			fsys.zipCache.Invalidate(key)
		}
	}
	fsys.lock.Lock()
	defer fsys.lock.Unlock()
	var newToClose []fs.FS
	for _, zipFS := range fsys.toClose {
		if !isLocked(zipFS) && getRefCount(zipFS) == 0 {
			writefs.Close(zipFS)
		} else {
			newToClose = append(newToClose, zipFS)
		}
	}
	fsys.toClose = newToClose
	return nil
}
func isZipFile(name string) bool {
	return strings.ToLower(filepath.Ext(name)) == ".zip"
}

func expandZipFile(name string) (zipFile string, zipPath string, isZip bool) {
	name = clearPath(name)
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
	_ writefs.CreateFS      = (*zipAsFolderFS)(nil)
	_ writefs.MkDirFS       = (*zipAsFolderFS)(nil)
	_ writefs.CloseFS       = (*zipAsFolderFS)(nil)
	_ writefs.FullpathFS    = (*zipAsFolderFS)(nil)
	_ writefs.EqualFS       = (*zipAsFolderFS)(nil)
	_ writefs.IsWriteableFS = (*zipAsFolderFS)(nil)
	_ fs.FS                 = (*zipAsFolderFS)(nil)
	_ fs.ReadDirFS          = (*zipAsFolderFS)(nil)
	_ fs.ReadFileFS         = (*zipAsFolderFS)(nil)
	_ fs.StatFS             = (*zipAsFolderFS)(nil)
)
