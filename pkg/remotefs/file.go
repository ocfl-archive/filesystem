package remotefs

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"emperror.dev/errors"
)

const rangeLength = 16 * 1024 * 1024

type file struct {
	emptyResultCounter uint
	d                  *remoteFSRW
	name               string
	rc                 io.ReadCloser
	url                string
	pos                int64
	size               int64 // -1: no information
	canRange           bool
	client             *http.Client
	token              string
	eof                bool
}

var contentRangeRegexp = regexp.MustCompile(`^bytes ([0-9]+-[0-9]+|\*)/([0-9*]+)$`)

func (f *file) init(pos int64) error {
	f.client = &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
		},
	}
	f.size = 0
	req, err := http.NewRequest(http.MethodGet, f.url, nil)
	if err != nil {
		return errors.Wrapf(err, "cannot create request for '%s'", f.url)
	}
	if pos > 0 {
		f.pos = pos
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", pos, pos+rangeLength-1))
	}
	if f.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", f.token))
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "cannot send request for '%s'", f.url)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return errors.Errorf("invalid status '%s' for '%s'", resp.Status, f.url)
	}
	f.rc = resp.Body
	if pos > 0 {
		if resp.StatusCode != http.StatusPartialContent {
			return errors.Errorf("invalid status for range request '%s' for '%s'", resp.Status, f.url)
		}
		contentRangeStr := resp.Header.Get("Content-Range")
		if contentRangeStr == "" {
			return errors.Errorf("no Content-Range header found for '%s'", f.url)
		}
		matches := contentRangeRegexp.FindStringSubmatch(contentRangeStr)
		if matches == nil {
			return errors.Errorf("invalid content range %s for '%s' with range '%s'", contentRangeStr, f.url, fmt.Sprintf("bytes=%d-%d", f.pos, min(f.pos+rangeLength-1, f.size-1)))
		}
		if matches[2] != "*" {
			f.size, err = strconv.ParseInt(matches[2], 10, 64)
			if err != nil {
				return errors.Wrapf(err, "invalid content range %s for '%s' with size '%s'", matches[3], "/", f.url)
			}
		}
		f.canRange = true
	} else {
		clHeader := resp.Header.Get("Content-Length")
		if clHeader != "" {
			size, err := strconv.ParseInt(clHeader, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "cannot parse content length '%s' for '%s'", clHeader, f.url)
			}
			f.size = size
			f.eof = f.size == 0
		}
		acceptRangesHeader := resp.Header.Get("Accept-Ranges")
		f.canRange = acceptRangesHeader == "bytes"
	}
	return nil
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = offset
	case io.SeekCurrent:
		f.pos += offset
	case io.SeekEnd:
		if f.size == -1 {
			return 0, errors.New("cannot seek to end of file because file size is unknown")
		}
		f.pos = f.size + offset
	default:
		return 0, errors.Errorf("unknown whence: %d", whence)
	}
	if f.rc != nil {
		f.rc.Close()
	}
	if f.client == nil {
		f.init(f.pos)
	}
	return f.pos, nil
}

func (f *file) Read(p []byte) (n int, err error) {
	if f.client == nil {
		if err := f.init(0); err != nil {
			return 0, errors.WithStack(err)
		}
	}
	if f.eof {
		return 0, io.EOF
	}
	if f.pos >= f.size && f.size != -1 {
		if f.rc != nil {
			f.rc.Close()
		}
		f.eof = true
		return 0, io.EOF
	}
	if f.rc == nil {
		req, err := http.NewRequest(http.MethodGet, f.url, nil)
		if err != nil {
			return 0, errors.Wrapf(err, "cannot create request for '%s'", f.url)
		}
		if f.canRange {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", f.pos, min(f.pos+rangeLength-1, f.size-1)))
		}
		if f.token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", f.token))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, errors.Wrapf(err, "cannot send request for '%s'", f.url)
		}
		if resp.StatusCode != http.StatusPartialContent {
			return 0, errors.Wrapf(err, "invalid response %s for '%s'", resp.Status, f.url)
		}
		contentLengthStr := resp.Header.Get("Content-Length")
		if contentLengthStr != "" {
			contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
			if err != nil {
				return 0, errors.Wrapf(err, "cannot parse content length result header '%s' for '%s'", contentLengthStr, f.url)
			}
			// todo: check contentLength
			_ = contentLength
		}
		contentRangeStr := resp.Header.Get("Content-Range")
		if contentRangeStr != "" {
			if matches := contentRangeRegexp.FindStringSubmatch(contentRangeStr); matches == nil {
				return 0, errors.Errorf("invalid content range %s for '%s' with range '%s'", contentRangeStr, f.url, fmt.Sprintf("bytes=%d-%d", f.pos, min(f.pos+rangeLength-1, f.size-1)))
			} else {
				// todo: check content range
			}
		}
		f.rc = resp.Body
	}
	n, err = f.rc.Read(p)
	f.pos += int64(n)
	if errors.Is(err, io.EOF) {
		err = nil
		f.rc.Close()
		f.rc = nil
		if !f.canRange {
			f.eof = true
			return n, io.EOF
		}
	}
	if n == 0 {
		f.emptyResultCounter++
		if f.emptyResultCounter >= 3 {
			return 0, errors.Errorf("too many empty results for '%s'", f.url)
		}
	} else {
		f.emptyResultCounter = 0
	}
	return n, errors.Wrapf(err, "cannot read file body for '%s'", f.url)
}

func (f *file) Close() error {
	if f.rc != nil {
		return errors.WithStack(f.rc.Close())
	}
	return nil
}

func (f *file) Stat() (info fs.FileInfo, err error) {
	return f.d.Stat(f.name)
}

type fileWrite struct {
	d    *remoteFSRW
	name string
	wc   io.WriteCloser
	done chan error
}

func (f *fileWrite) Write(p []byte) (n int, err error) {
	return f.wc.Write(p)
}

func (f *fileWrite) Close() error {
	if err := f.wc.Close(); err != nil {
		return err
	}
	select {
	case err := <-f.done:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("timeout")
	}
}

var _ fs.File = (*file)(nil)
var _ io.Seeker = (*file)(nil)
