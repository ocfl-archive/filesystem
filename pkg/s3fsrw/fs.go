package s3fsrw

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"golang.org/x/exp/slices"
)

func NewFS(endpoint, accessKeyID, secretAccessKey, region string, useSSL, debug bool, tlsConfig *tls.Config, dnsNetwork, dnsAddress string, readOnly bool, logger zLogger.ZLogger) (*s3FSRW, error) {
	_logger := logger.With().Str("class", "s3FSRW").Logger()
	var err error
	fSys := &s3FSRW{
		client:   nil,
		readOnly: readOnly,
		//		bucket:   bucket,
		region:   region,
		endpoint: endpoint,
		logger:   zLogger.NewZWrapper(&_logger),
	}

	var tr http.RoundTripper = &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	if dnsAddress != "" {
		if dnsNetwork == "" {
			dnsNetwork = "udp"
		}
		logger.Debug().Msgf("using DNS resolver %s:%s", dnsNetwork, dnsAddress)
		d := net.Dialer{}
		dialer := &net.Dialer{
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					return d.DialContext(ctx, dnsNetwork, dnsAddress)
				},
			},
		}
		dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		}
		tr.(*http.Transport).DialContext = dialContext
	}

	if debug {
		tr = NewDebuggingRoundTripper(
			&http.Transport{
				TLSClientConfig: tlsConfig,
			},
			zLogger.NewZWrapper(&_logger),
			JustURL,
			URLTiming,
			// CurlCommand,
			RequestHeaders,
			ResponseStatus,
			// ResponseHeaders,
		)
	}
	fSys.client, err = minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure:    useSSL,
		Region:    region,
		Transport: tr,
	})
	if err != nil {
		return nil, errors.Wrap(err, "cannot create s3 client instance")
	}
	return fSys, nil
}

type s3FSRW struct {
	client   *minio.Client
	region   string
	endpoint string
	logger   zLogger.ZWrapper
	readOnly bool
}

func (s3FS *s3FSRW) IsEmpty(dir string) (bool, error) {
	entries, err := s3FS.ReadDir(dir)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return len(entries) == 0, nil
}

func (s3FS *s3FSRW) SubCreate(dir string) (fs.FS, error) {
	return s3FS.Sub(dir)
}

func (s3FS *s3FSRW) RealPath(path string) string {
	return path
}

func (s3FS *s3FSRW) Equal(fsys fs.FS) bool {
	if s3FS2, ok := fsys.(*s3FSRW); ok {
		return s3FS.endpoint == s3FS2.endpoint && s3FS.region == s3FS2.region && s3FS.readOnly == s3FS2.readOnly
	}
	return false
}

func (s3FS *s3FSRW) Close() error {
	return nil
}

func (s3FS *s3FSRW) WriteFile(path string, data []byte) (int64, error) {
	if s3FS.readOnly {
		return 0, errors.New("read-only filesystem")
	}
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Create(%s)", s3FS.String(), path)
	}
	ctx := context.Background()
	byteBuffer := bytes.NewBuffer(data)
	ui, err := s3FS.client.PutObject(ctx, bucket, bucketPath, byteBuffer, -1, minio.PutObjectOptions{})
	if err != nil {
		return 0, errors.Wrapf(err, "cannot write '%s'", path)
	}
	return ui.Size, nil
}

func (s3FS *s3FSRW) IsWriteable(path string) bool {
	return !s3FS.readOnly
}

func (s3FS *s3FSRW) Fullpath(name string) (string, error) {
	return name, nil
}

// MkDir does nothing
func (s3FS *s3FSRW) MkDir(path string) error {
	if s3FS.readOnly {
		return errors.New("read-only filesystem")
	}
	bucket, bucketPath := extractBucket(path)
	if bucketPath != "" {
		return errors.Wrapf(fs.ErrInvalid, "cannot create bucket with subfolders '%s'", path)
	}
	return errors.Wrapf(s3FS.client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{Region: s3FS.region}), "cannot create bucket '%s'", bucket)
}

func (s3FS *s3FSRW) Open(path string) (fs.File, error) {
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Open(%s)", s3FS.String(), path)
	}
	ctx := context.Background()
	object, err := s3FS.client.GetObject(ctx, bucket, bucketPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open '%s/%s/%s'", s3FS.client.EndpointURL(), bucket, path)
	}
	objectInfo, err := object.Stat()
	if err != nil {
		object.Close()
		if s3FS.IsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, errors.Wrapf(err, "cannot stat '%s'", path)
	}
	if objectInfo.Err != nil {
		object.Close()
		return nil, errors.Wrapf(objectInfo.Err, "error in objectInfo of '%s'", path)
	}
	return NewROFile(object, path, s3FS.logger), nil
}

func (s3FS *s3FSRW) ReadFile(path string) ([]byte, error) {
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - ReadFile(%s)", s3FS.String(), path)
	}
	fp, err := s3FS.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open '%s'", path)
	}
	defer fp.Close()
	data := bytes.NewBuffer(nil)
	if _, err := io.Copy(data, fp); err != nil {
		return nil, errors.Wrapf(err, "cannot read '%s'", path)
	}
	return data.Bytes(), nil
}

func (s3FS *s3FSRW) Copy(src, dst string) (int64, error) {
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Copy(%s, %s)", s3FS.String(), src, dst)
	}
	dstBucket, dstBucketPath := extractBucket(dst)
	srcBucket, srcBucketPath := extractBucket(src)
	ui, err := s3FS.client.CopyObject(context.Background(),
		minio.CopyDestOptions{Bucket: dstBucket, Object: dstBucketPath},
		minio.CopySrcOptions{Bucket: srcBucket, Object: srcBucketPath})
	if err != nil {
		return 0, errors.Wrapf(err, "cannot copy '%s' --> '%s'", src, dst)
	}
	return ui.Size, nil
}

func (s3FS *s3FSRW) ReadDir(path string) ([]fs.DirEntry, error) {
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - ReadDir(%s)", s3FS.String(), path)
	}
	if bucket == "" {
		bucketInfo, err := s3FS.client.ListBuckets(context.Background())
		if err != nil {
			return nil, errors.Wrapf(err, "cannot list buckets")
		}
		result := []fs.DirEntry{}
		for _, bi := range bucketInfo {
			result = append(result, writefs.NewDirEntry(writefs.NewFileInfoDir(bi.Name)))
		}
		return result, nil
	}
	ctx := context.Background()
	result := []fs.DirEntry{}

	for objectInfo := range s3FS.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: bucketPath}) {
		if objectInfo.Err != nil {
			return nil, errors.Wrapf(objectInfo.Err, "cannot read '%s'", path)
		}
		result = append(result, writefs.NewDirEntry(NewFileInfo(new(objectInfo))))
	}
	return result, nil
}

func (s3FS *s3FSRW) Create(path string) (writefs.FileWrite, error) {
	if s3FS.readOnly {
		return nil, errors.New("read-only filesystem")
	}
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Create(%s)", s3FS.String(), path)
	}
	ctx := context.Background()
	wc := NewWriteCloser(path, s3FS.logger)
	go func() {
		ui, err := s3FS.client.PutObject(ctx, bucket, bucketPath, wc.GetReader(), -1, minio.PutObjectOptions{})
		uierr := NewUploadInfo(&ui, err)
		wc.c <- uierr
		if err != nil {
			wc.Close()
		}
	}()
	return wc, nil
}

func (s3FS *s3FSRW) Append(path string) (writefs.FileWrite, error) {
	if s3FS.readOnly {
		return nil, errors.New("read-only filesystem")
	}
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Create(%s)", s3FS.String(), path)
	}
	ctx := context.Background()
	wc := NewWriteCloser(path, s3FS.logger)
	go func() {
		// new data is in .append
		ui, err := s3FS.client.PutObject(ctx, bucket, bucketPath+".append", wc.GetReader(), -1, minio.PutObjectOptions{})
		var errs []error
		if err != nil {
			errs = append(errs, err)
		} else {
			// .appendall is the concatenation of the original file and the new data
			if ui, err = s3FS.client.ComposeObject(ctx,
				minio.CopyDestOptions{Bucket: bucket, Object: bucketPath + ".appendall"},
				minio.CopySrcOptions{Bucket: bucket, Object: bucketPath},
				minio.CopySrcOptions{Bucket: bucket, Object: bucketPath + ".append"}); err != nil {
				errs = append(errs, err)
			} else {
				// copy .appendall to original file
				if ui, err = s3FS.client.CopyObject(ctx, minio.CopyDestOptions{
					Bucket: bucket,
					Object: bucketPath,
				}, minio.CopySrcOptions{
					Bucket: bucket,
					Object: bucketPath + ".appendall",
				}); err != nil {
					errs = append(errs, err)
				} else {
					// remove temporary files
					if err = s3FS.client.RemoveObject(ctx, bucket, bucketPath+".append", minio.RemoveObjectOptions{}); err != nil {
						errs = append(errs, err)
					}
					if err = s3FS.client.RemoveObject(ctx, bucket, bucketPath+".appendall", minio.RemoveObjectOptions{}); err != nil {
						errs = append(errs, err)
					}
				}
			}
		}
		if len(errs) > 0 {
			wc.Close()
		}
		uierr := NewUploadInfo(&ui, errors.Combine(errs...))
		wc.c <- uierr
	}()
	return wc, nil
}

func (s3FS *s3FSRW) Remove(path string) error {
	if s3FS.readOnly {
		return errors.New("read-only filesystem")
	}
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Delete(%s)", s3FS.String(), path)
	}
	ctx := context.Background()
	if err := s3FS.client.RemoveObject(ctx, bucket, bucketPath, minio.RemoveObjectOptions{}); err != nil {
		if s3FS.IsNotExist(err) {
			return fs.ErrNotExist
		}
		return errors.Wrapf(err, "cannot remove '%s'", path)
	}
	return nil
}

func (s3FS *s3FSRW) Sub(subfolder string) (fs.FS, error) {
	return writefs.Sub(s3FS, subfolder)
}

func (s3FS *s3FSRW) String() string {
	return s3FS.endpoint
}

func (s3FS *s3FSRW) Rename(src, dest string) error {
	if s3FS.readOnly {
		return errors.New("read-only filesystem")
	}
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - Rename(%s, %s)", s3FS.String(), src, dest)
	}
	_, err := s3FS.Stat(dest)
	if err != nil {
		if !s3FS.IsNotExist(err) {
			return errors.Wrapf(err, "cannot stat '%s'", dest)
		}
	} else {
		if err := s3FS.Remove(dest); err != nil {
			return errors.Wrapf(err, "cannot delete '%s'", dest)
		}
	}
	// now, dest should not exist...

	srcFP, err := s3FS.Open(src)
	if err != nil {
		return errors.Wrapf(err, "cannot open '%s'", src)
	}
	defer srcFP.Close()
	destFP, err := s3FS.Create(dest)
	if err != nil {
		return errors.Wrapf(err, "cannot create '%s'", dest)
	}
	defer destFP.Close()
	if _, err := io.Copy(destFP, srcFP); err != nil {
		return errors.Wrapf(err, "cannot copy '%s' --> '%s'", src, dest)
	}
	return nil
}

var notFoundStatus = []int{
	http.StatusNotFound,
	// http.StatusForbidden,
	// http.StatusConflict,
	// http.StatusPreconditionFailed,
}

func (s3FS *s3FSRW) IsNotExist(err error) bool {
	errResp, ok := err.(minio.ErrorResponse)
	if !ok {
		return false
	}
	return slices.Contains(notFoundStatus, errResp.StatusCode)
}

func (s3FS *s3FSRW) WalkDir(path string, fn fs.WalkDirFunc) error {
	var err error
	bucket, bucketPath := extractBucket(path)
	if s3FS.logger != nil {
		s3FS.logger.Debugf("%s - WalkDir(%s)", s3FS.String(), path)
	}
	var bucketEntries []fs.DirEntry
	if bucket == "" {
		bucketEntries, err = s3FS.ReadDir("")
		if err != nil {
			return errors.Wrapf(err, "cannot list buckets")
		}
	} else {
		bucketEntries = []fs.DirEntry{writefs.NewDirEntry(writefs.NewFileInfoDir(bucket))}
	}
	for _, bucketEntry := range bucketEntries {
		ctx := context.Background()
		for objectInfo := range s3FS.client.ListObjects(ctx, bucketEntry.Name(), minio.ListObjectsOptions{
			Prefix:    bucketPath,
			Recursive: true,
		}) {
			if err := fn(objectInfo.Key, writefs.NewDirEntry(NewFileInfo(&objectInfo)), nil); err != nil {
				return errors.Wrapf(err, "error in '%s'", objectInfo.Key)
			}
		}

	}
	return nil
}

func (s3FS *s3FSRW) Stat(path string) (fs.FileInfo, error) {
	bucket, bucketPath := extractBucket(path)
	if bucket == "" {
		return writefs.NewFileInfoDir(path), nil
	}
	ctx := context.Background()
	objectInfo, err := s3FS.client.StatObject(ctx, bucket, bucketPath, minio.StatObjectOptions{})
	if err != nil {
		if s3FS.IsNotExist(err) {
			if s3FS.hasContent(path) {
				return writefs.NewFileInfoDir(path), nil
			}
			return nil, fs.ErrNotExist
		}
		return nil, errors.Wrapf(err, "cannot stat '%s'", path)
	}
	return &fileInfo{&objectInfo}, nil
}

func (s3FS *s3FSRW) hasContent(prefix string) bool {
	bucket, bucketPath := extractBucket(prefix)
	s3FS.logger.Debugf("%s - hasContent(%s)", s3FS.String(), prefix)
	ctx, cancel := context.WithCancel(context.Background())
	chanObjectInfo := s3FS.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: bucketPath})
	objectInfo, ok := <-chanObjectInfo
	if ok {
		if objectInfo.Err != nil {
			cancel()
			return true
		}
	}
	cancel()
	return ok
}

func (s3FS *s3FSRW) HasContent() bool {
	return s3FS.hasContent("")
}

var (
	_ fmt.Stringer   = &s3FSRW{}
	_ writefs.FullFS = &s3FSRW{}
)
