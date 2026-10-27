package zipasfolder

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/maypok86/otter/v2"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"golang.org/x/sync/singleflight"
)

type fileCloser struct {
	fs.File
	closeFunc func() error
	closeOnce sync.Once
	closeErr  error
}

func (fc *fileCloser) Close() error {
	fc.closeOnce.Do(func() {
		err := fc.File.Close()
		if fc.closeFunc != nil {
			if err2 := fc.closeFunc(); err2 != nil {
				if err == nil {
					err = err2
				}
			}
		}
		fc.closeErr = err
	})
	return fc.closeErr
}

// NewFS creates a new zipAsFolderFS which handles zipfiles like folders which are read-only
// it implements readwritefs.ReadWriteFS, fs.ReadDirFS, fs.ReadFileFS, basefs.CloserFS
func NewFS(baseFS fs.FS, cacheSize int, timeout time.Duration, readOnly bool, logger zLogger.ZLogger) (*zipAsFolderFS, error) {
	if timeout <= 0 {
		timeout = time.Minute
	}
	logger = new(logger.With().Str("class", "zipAsFolderFS").Logger())
	ctx, cancel := context.WithCancel(context.Background())
	f := &zipAsFolderFS{
		baseFS:   baseFS,
		readOnly: readOnly,
		cancel:   cancel,
		logger:   logger,
		toClose:  []fs.FS{},
	}
	zipCache, err := otter.New[string, fs.FS](&otter.Options[string, fs.FS]{
		MaximumSize: cacheSize,
		OnAtomicDeletion: func(e otter.DeletionEvent[string, fs.FS]) {
			// This function is called when an item is removed from the cache (evicted or invalidated).
			zipFS := e.Value
			f.toCloseLock.Lock()
			rc := getRefCount(zipFS)
			if isLocked(zipFS) || rc > 0 {
				f.toClose = append(f.toClose, zipFS)
				f.toCloseLock.Unlock()
			} else {
				f.toCloseLock.Unlock()
				writefs.Close(zipFS)
			}
		},
	})
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "cannot create otter cache")
	}
	f.zipCache = zipCache
	go func() {
		// This goroutine periodically checks for zip files that are no longer in use
		// and can be safely closed. This includes files in the 'toClose' list.
		ticker := time.NewTicker(timeout)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f.ClearUnlocked()
			}
		}
	}()
	// runtime.AddCleanup ensures that the background goroutine is stopped
	// when the zipAsFolderFS object is garbage collected.
	runtime.AddCleanup(f, func(cancelFunc context.CancelFunc) {
		cancelFunc()
	}, cancel)
	return f, nil
}

type zipAsFolderFS struct {
	baseFS      fs.FS
	zipCache    *otter.Cache[string, fs.FS]
	sf          singleflight.Group
	cancel      context.CancelFunc
	closeOnce   sync.Once
	logger      zLogger.ZLogger
	readOnly    bool
	toCloseLock sync.Mutex
	toClose     []fs.FS
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

// Create creates a new file
func (fsys *zipAsFolderFS) Create(path string) (writefs.FileWrite, error) {
	if fsys.readOnly {
		return nil, errors.New("read-only filesystem")
	}
	path = clearPath(path)
	zipFile, zipPath, isZIP := expandZipFile(path)
	if isZIP && zipPath != "" {
		return nil, errors.Errorf("cannot create file '%s' in zip file '%s'", path, zipFile)
	}
	return writefs.Create(fsys.baseFS, zipFile)
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
	return writefs.MkDir(fsys.baseFS, zipFile)
}

// getZipFS retrieves a zip filesystem from the cache or loads it if not present.
// It increases the reference count of the returned filesystem.
func (fsys *zipAsFolderFS) getZipFS(zipFile string) (fs.FS, error) {
	// Check if already in cache and not closed
	if zipFS, ok := fsys.zipCache.GetIfPresent(zipFile); ok {
		incRef(zipFS)
		if !isClosed(zipFS) {
			return zipFS, nil
		}
		decRef(zipFS)
	}

	// Use singleflight to deduplicate concurrent loading of the same archive
	v, err, _ := fsys.sf.Do(zipFile, func() (any, error) {
		// Double-check cache inside singleflight
		if zipFS, ok := fsys.zipCache.GetIfPresent(zipFile); ok {
			if !isClosed(zipFS) {
				return zipFS, nil
			}
		}

		// Load zip file from base filesystem
		fsys.logger.Debug().Msgf("load zip file '%s'", zipFile)
		f, err := fsys.baseFS.Open(zipFile)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot open zip file '%s'", zipFile)
		}
		// Create a new zip filesystem wrapper that supports reference counting and closing
		zipFS, err := NewZipFSCloser(f, zipFile, fsys.logger)
		if err != nil {
			fsys.logger.Debug().Msgf("load zip file '%s' FAILED: %v", zipFile, err)
			return nil, errors.Wrapf(err, "cannot create zipfscloser for '%s'", zipFile)
		}
		fsys.zipCache.Set(zipFile, zipFS)
		return zipFS, nil
	})
	if err != nil {
		return nil, err
	}
	zipFS := v.(fs.FS)
	incRef(zipFS)
	if isClosed(zipFS) {
		decRef(zipFS)
		return fsys.getZipFS(zipFile)
	}
	return zipFS, nil
}

// Stat returns the file info for a given path
func (fsys *zipAsFolderFS) Stat(name string) (fs.FileInfo, error) {
	fsys.logger.Debug().Msgf("Stat('%s')", name)
	name = clearPath(name)
	zipFile, zipPath, isZIP := expandZipFile(name)
	fsys.logger.Debug().Msgf("expandZipFile('%s') -> zipFile='%s', zipPath='%s', isZIP=%v", name, zipFile, zipPath, isZIP)
	if !isZIP {
		if zipFile == "" {
			zipFile = "."
		}
		info, err := fs.Stat(fsys.baseFS, zipFile)
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
		if zipFile == "" {
			zipFile = "."
		}
		entries, err := fs.ReadDir(fsys.baseFS, zipFile)
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

// Sub returns an fs.FS corresponding to the subtree rooted at name.
func (fsys *zipAsFolderFS) Sub(name string) (fs.FS, error) {
	fsys.logger.Debug().Msgf("Sub('%s')", name)
	name = clearPath(name)
	return writefs.NewSubFS(fsys, name)
}

// Open opens a file from the filesystem
func (fsys *zipAsFolderFS) Open(name string) (fs.File, error) {
	fsys.logger.Debug().Msgf("Open('%s')", name)
	name = clearPath(name)
	zipFile, zipPath, isZIP := expandZipFile(name)
	fsys.logger.Debug().Msgf("expandZipFile('%s') -> zipFile='%s', zipPath='%s', isZIP=%v", name, zipFile, zipPath, isZIP)
	if !isZIP {
		file, err := fsys.baseFS.Open(zipFile)
		if err != nil {
			return file, errors.Wrapf(err, "cannot open file '%s'", name)
		}
		return file, nil
	}

	if zipPath == "" {
		return fsys.baseFS.Open(zipFile)
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
	fsys.closeOnce.Do(func() {
		fsys.cancel()
		fsys.zipCache.InvalidateAll()
		fsys.ClearUnlocked()

		if closer, ok := fsys.baseFS.(io.Closer); ok {
			closer.Close()
		}
	})
	return nil
}

// ClearUnlocked iterates through the cache and the 'toClose' list to close
// zip files that are no longer referenced and not locked.
func (fsys *zipAsFolderFS) ClearUnlocked() error {
	// 1. Check all items currently in the cache.
	// If they are not locked and have no active references, we can remove them.
	for key, value := range fsys.zipCache.All() {
		if !isLocked(value) && getRefCount(value) == 0 {
			fsys.zipCache.Invalidate(key) // This triggers OnAtomicDeletion
		}
	}

	fsys.toCloseLock.Lock()
	defer fsys.toCloseLock.Unlock()

	// 2. Check the 'toClose' list.
	// This list contains zip files that were evicted from the cache but were still in use.
	var newToClose []fs.FS
	for _, zipFS := range fsys.toClose {
		if !isLocked(zipFS) && getRefCount(zipFS) == 0 {
			// Now safe to close
			writefs.Close(zipFS)
		} else {
			// Still in use or locked, keep in list for next cycle
			newToClose = append(newToClose, zipFS)
		}
	}
	fsys.toClose = newToClose
	return nil
}
func isZipFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".zip#zip")
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
			if before, ok := strings.CutSuffix(zipFile, "#zip"); ok {
				zipFile = before
				if zipPath == "" {
					return zipFile, "", false
				}
			}
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
