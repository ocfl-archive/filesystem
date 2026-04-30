package aferoFS

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/spf13/afero"
)

func NewCreateFSFunc(logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, baseFolder string, readOnly bool) (fs.FS, error) {
		var aferoFS afero.Fs
		u, err := url.Parse(baseFolder)
		if err != nil {
			return nil, err
		}

		switch u.Scheme {
		case "mem":
			aferoFS = afero.NewMemMapFs()
		case "file":
			folder := strings.TrimPrefix(baseFolder, "file://")
			aferoFS = afero.NewBasePathFs(afero.NewOsFs(), folder)
		case "base":
			// base://[basepath]?[options]
			basePath := u.Path
			if u.Host != "" {
				basePath = u.Host + u.Path
			}
			aferoFS = afero.NewBasePathFs(afero.NewOsFs(), basePath)

		case "copyonwrite", "cow":
			// cow://?base=[base_url]&layer=[layer_url]
			base := u.Query().Get("base")
			layer := u.Query().Get("layer")
			if base == "" || layer == "" {
				return nil, fmt.Errorf("copyonwrite filesystem requires 'base' and 'layer' parameters")
			}
			baseFS, err := f.Get(base, readOnly)
			if err != nil {
				return nil, err
			}
			layerFS, err := f.Get(layer, readOnly)
			if err != nil {
				return nil, err
			}
			// check if baseFS/layerFS are already *aferoFSRW and extract underlying afero.Fs
			var bFS, lFS afero.Fs
			if afs, ok := baseFS.(interface{ GetAfero() afero.Fs }); ok {
				bFS = afs.GetAfero()
			} else {
				return nil, fmt.Errorf("base filesystem must be an afero.Fs for copyonwrite")
			}
			if afs, ok := layerFS.(interface{ GetAfero() afero.Fs }); ok {
				lFS = afs.GetAfero()
			} else {
				return nil, fmt.Errorf("layer filesystem must be an afero.Fs for copyonwrite")
			}
			aferoFS = afero.NewCopyOnWriteFs(bFS, lFS)

		case "readonly", "ro":
			// ro://?base=[base_url]
			base := u.Query().Get("base")
			if base == "" {
				return nil, fmt.Errorf("readonly filesystem requires 'base' parameter")
			}
			baseFS, err := f.Get(base, true)
			if err != nil {
				return nil, err
			}
			if afs, ok := baseFS.(interface{ GetAfero() afero.Fs }); ok {
				aferoFS = afero.NewReadOnlyFs(afs.GetAfero())
			} else {
				return nil, fmt.Errorf("base filesystem must be an afero.Fs for readonly")
			}

		case "regexp":
			// regexp://?base=[base_url]&re=[regex]
			base := u.Query().Get("base")
			reStr := u.Query().Get("re")
			if base == "" || reStr == "" {
				return nil, fmt.Errorf("regexp filesystem requires 'base' and 're' parameters")
			}
			baseFS, err := f.Get(base, readOnly)
			if err != nil {
				return nil, err
			}
			re, err := regexp.Compile(reStr)
			if err != nil {
				return nil, fmt.Errorf("invalid regexp '%s': %w", reStr, err)
			}
			if afs, ok := baseFS.(interface{ GetAfero() afero.Fs }); ok {
				aferoFS = afero.NewRegexpFs(afs.GetAfero(), re)
			} else {
				return nil, fmt.Errorf("base filesystem must be an afero.Fs for regexp")
			}

		case "http", "https":
			// http://host/path?params
			// We can use afero.NewHttpFs but it's for exposing afero as http.FileSystem.
			// For remote HTTP as afero.Fs, it's not in standard afero.
			// But we could support it if we had a proper implementation.
			// For now, let's just use it as a BasePath OS FS if it's not a real URL
			// or keep the default behavior.
			fallthrough

		default:
			// Default to OS FS with base path if no scheme or unknown scheme
			folder := baseFolder
			if u.Scheme != "" && (u.Scheme == "os" || strings.Contains(baseFolder, "://")) {
				folder = strings.TrimPrefix(baseFolder, u.Scheme+"://")
			}
			if folder == "" {
				folder = "."
			}
			// convert to absolute path if possible
			if absFolder, err := filepath.Abs(folder); err == nil {
				folder = absFolder
			}
			if _, err := os.Stat(folder); err != nil {
				if os.IsNotExist(err) {
					if err := os.MkdirAll(folder, 0755); err != nil {
						return nil, fmt.Errorf("cannot create folder '%s': %w", folder, err)
					}
				} else {
					return nil, fmt.Errorf("cannot stat folder '%s': %w", folder, err)
				}
			}

			aferoFS = afero.NewBasePathFs(afero.NewOsFs(), folder)
		}

		if readOnly {
			aferoFS = afero.NewReadOnlyFs(aferoFS)
		}
		return NewFS(aferoFS, logger)
	}
}
