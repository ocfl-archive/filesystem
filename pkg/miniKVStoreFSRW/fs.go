package miniKVStoreFSRW

import (
	"bytes"
	"context"
	"emperror.dev/errors"
	"fmt"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	genericproto "go.ub.unibas.ch/cloud/genericproto/v2/pkg/generic/proto"
	"go.ub.unibas.ch/cloud/minikvstore/pkg/minikvstoreproto"
	"go.ub.unibas.ch/cloud/miniresolver/v2/pkg/resolver"
	"io"
	"io/fs"
	"path"
	"strings"
)

func splitBucketDir(fullpath string) (bucket, dir string) {
	parts := strings.SplitN(fullpath, "/", 2)
	if len(parts) < 2 {
		return fullpath, ""
	}
	bucket = parts[0]
	dir = parts[1]
	return bucket, dir
}

func NewFS(miniResolverClient *resolver.MiniResolver, domain, vfs, dir string, closer []io.Closer, readOnly bool, logger zLogger.ZLogger) (*miniKVFSRW, error) {
	_logger := logger.With().Str("class", "miniKVFSRW").Logger()
	logger = &_logger

	miniKVClient, err := resolver.NewClient[minikvstoreproto.MiniKVStoreClient](
		miniResolverClient,
		minikvstoreproto.NewMiniKVStoreClient,
		minikvstoreproto.MiniKVStore_ServiceDesc.ServiceName,
		domain,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create miniKVStore clients for domain %s", domain)
	}
	return &miniKVFSRW{
		client:   miniKVClient,
		domain:   domain,
		vfs:      vfs,
		dir:      dir,
		readOnly: readOnly,
		close:    closer,
		logger:   logger,
	}, nil
}

type miniKVFSRW struct {
	logger   zLogger.ZLogger
	close    []io.Closer
	readOnly bool
	client   minikvstoreproto.MiniKVStoreClient
	vfs      string
	dir      string
	domain   string
}

func (d *miniKVFSRW) Equal(fsys fs.FS) bool {
	if fsys2, ok := fsys.(*miniKVFSRW); ok {
		return d.domain == fsys2.domain &&
			d.vfs == fsys2.vfs &&
			d.dir == fsys2.dir &&
			d.readOnly == fsys2.readOnly
	}
	return false
}

func (d *miniKVFSRW) Fullpath(name string) (string, error) {
	return fmt.Sprintf("vfs://%s", path.Join(d.vfs, d.dir, name)), nil
}

func (d *miniKVFSRW) Close() error {
	var errs = []error{}
	for _, c := range d.close {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Combine(errs...)
}

func (d *miniKVFSRW) String() string {
	return "miniKVFSRW(" + d.vfs + ")"
}

func (d *miniKVFSRW) Sub(dir string) (fs.FS, error) {
	return &miniKVFSRW{
		client:   d.client,
		readOnly: d.readOnly,
		vfs:      d.vfs,
		logger:   d.logger,
		dir:      path.Join(d.dir, dir),
	}, nil
}

func (d *miniKVFSRW) Remove(filename string) error {
	if d.readOnly {
		return errors.New("read only filesystem")
	}
	filename = path.Join(d.dir, filename)
	bucket, dir := splitBucketDir(filename)
	resp, err := d.client.Delete(context.TODO(), &minikvstoreproto.KeyData{
		Key:    dir,
		Bucket: bucket,
	})
	if err != nil {
		return errors.Wrapf(err, "cannot delete '%s/%s'", bucket, dir)
	}
	if resp.Status != genericproto.ResultStatus_OK {
		return errors.Errorf("cannot delete '%s/%s': [%s] %s", bucket, dir, resp.Status.String(), resp.Message)
	}
	return nil
}

func (d *miniKVFSRW) Rename(oldPath, newPath string) error {
	if d.readOnly {
		return errors.New("read only filesystem")
	}
	return errors.Errorf("rename not supported for miniKVFSRW")
}

func (d *miniKVFSRW) Open(filename string) (fs.File, error) {
	filename = path.Join(d.dir, filename)
	bucket, dir := splitBucketDir(filename)
	resp, err := d.client.Get(context.TODO(), &minikvstoreproto.KeyData{
		Key:    dir,
		Bucket: bucket,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get '%s/%s'", bucket, dir)
	}
	fp := &file{
		data:   bytes.NewBuffer(resp.Value),
		length: len(resp.Value),
		name:   filename,
	}
	return fp, nil
}

func (d *miniKVFSRW) Stat(filename string) (fs.FileInfo, error) {
	filename = path.Join(d.dir, filename)
	// TODO: optimize with cache
	bucket, dir := splitBucketDir(filename)
	resp, err := d.client.Get(context.TODO(), &minikvstoreproto.KeyData{
		Key:    dir,
		Bucket: bucket,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get '%s/%s'", bucket, dir)
	}
	fp := &file{
		data:   bytes.NewBuffer(resp.Value),
		length: len(resp.Value),
		name:   filename,
	}
	return fp.Stat()
}

func (d *miniKVFSRW) Create(filename string) (writefs.FileWrite, error) {
	if d.readOnly {
		return nil, errors.New("read only filesystem")
	}
	filename = path.Join(d.dir, filename)
	fp := &fileWrite{
		client: d.client,
		name:   filename,
		data:   bytes.Buffer{},
	}
	return fp, nil
}

func (d *miniKVFSRW) ReadFile(filename string) ([]byte, error) {
	fp, err := d.Open(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open '%s'", filename)
	}
	defer fp.Close()
	return io.ReadAll(fp)
}

var (
	_ writefs.EqualFS     = &miniKVFSRW{}
	_ writefs.CreateFS    = &miniKVFSRW{}
	_ writefs.ReadWriteFS = &miniKVFSRW{}
	//_ writefs.MkDirFS     = &miniKVFSRW{}
	_ writefs.RenameFS   = &miniKVFSRW{}
	_ writefs.RemoveFS   = &miniKVFSRW{}
	_ writefs.FullpathFS = &miniKVFSRW{}
	//_ fs.ReadDirFS        = &miniKVFSRW{}
	_ fs.ReadFileFS = &miniKVFSRW{}
	_ fs.StatFS     = &miniKVFSRW{}
	_ fs.SubFS      = &miniKVFSRW{}
)
