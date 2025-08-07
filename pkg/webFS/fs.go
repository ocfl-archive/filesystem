package webFS

import (
	"crypto/tls"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
)

func NewFS(baseuri string, header map[string][]string, tlsInsecureSkipVerify bool, logger zLogger.ZLogger) (*webFSRW, error) {
	_logger := logger.With().Str("class", "webFSRW").Logger()
	logger = &_logger

	httpHeader := http.Header{}
	for k, vs := range header {
		for _, v := range vs {
			httpHeader.Add(k, v)
		}
	}
	return &webFSRW{
		header:                httpHeader,
		baseuri:               baseuri,
		logger:                logger,
		tlsInsecureSkipVerify: tlsInsecureSkipVerify,
	}, nil
}

type webFSRW struct {
	baseuri               string
	basename              string
	logger                zLogger.ZLogger
	header                http.Header
	tlsInsecureSkipVerify bool
}

func (d *webFSRW) Copy(dst, src string) (int64, error) {
	return 0, errors.New("read only filesystem")
}

func (d *webFSRW) Equal(fsys fs.FS) bool {
	if ofsys, ok := fsys.(*webFSRW); ok {
		return d.baseuri == ofsys.baseuri
	}
	return false
}

func (d *webFSRW) buildURL(name string) string {
	name = url.QueryEscape(path.Join(d.basename, path.Clean(filepath.ToSlash(name))))
	urlStr := strings.ReplaceAll(d.baseuri, "%%PATH%%", name)
	return urlStr
}

func (d *webFSRW) Fullpath(name string) (string, error) {
	return d.buildURL(name), nil
}

func (d *webFSRW) String() string {
	return "webFSRW(" + d.baseuri + ")"
}

func (d *webFSRW) query(urlStr string) (*http.Response, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create request for '%s'", urlStr)
	}
	req.Header = d.header
	if d.tlsInsecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s'", urlStr)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, errors.Errorf("cannot open file '%s': %s", urlStr, resp.Status)
	}
	return resp, nil
}

func (d *webFSRW) Open(name string) (fs.File, error) {
	urlStr := d.buildURL(name)
	resp, err := d.query(urlStr)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s'", urlStr)
	}
	return &file{
		Response: resp,
	}, nil
}

func (d *webFSRW) Stat(name string) (fs.FileInfo, error) {
	urlStr := d.buildURL(name)
	resp, err := d.query(urlStr)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open file '%s'", urlStr)
	}
	return &file{
		Response: resp,
	}, nil
}

func (d *webFSRW) ReadFile(name string) ([]byte, error) {
	fp, err := d.Open(name)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read file '%s'", name)
	}
	defer fp.Close()
	data, err := io.ReadAll(fp)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read file '%s'", name)
	}
	return data, nil
}

func (d *webFSRW) Close() error {
	return nil
}

var (
	_ fs.FS         = &webFSRW{}
	_ fs.ReadFileFS = &webFSRW{}
	_ fs.StatFS     = &webFSRW{}
)
