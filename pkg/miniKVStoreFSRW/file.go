package miniKVStoreFSRW

import (
	"bytes"
	"context"
	"io/fs"

	"emperror.dev/errors"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	genericproto "go.ub.unibas.ch/cloud/genericproto/v2/pkg/generic/proto"
	"go.ub.unibas.ch/cloud/minikvstore/pkg/minikvstoreproto"
)

type file struct {
	data   *bytes.Buffer
	length int
	name   string
}

func (f *file) Read(p []byte) (n int, err error) {
	return f.data.Read(p)
}

func (f *file) Close() error {
	return nil
}

func (f *file) Stat() (info fs.FileInfo, err error) {
	return &fileInfo{
		Name_:    f.name,
		Size_:    int64(f.length),
		Mode_:    fs.ModePerm, // assuming regular file with read/write permissions
		ModTime_: "",
		IsDir_:   false,
	}, nil
}

type fileWrite struct {
	client minikvstoreproto.MiniKVStoreClient
	name   string
	data   bytes.Buffer
}

func (f *fileWrite) Write(p []byte) (n int, err error) {
	return f.data.Write(p)
}

func (f *fileWrite) Close() error {
	bucket, dir := splitBucketDir(f.name)
	resp, err := f.client.Set(context.TODO(), &minikvstoreproto.ValueData{
		Key: &minikvstoreproto.KeyData{
			Bucket: bucket,
			Key:    dir,
		},
		Value:    f.data.Bytes(),
		Metadata: nil,
	})
	if err != nil {
		return errors.Wrapf(err, "error writing file %s", f.name)
	}
	if resp.Status != genericproto.ResultStatus_OK {
		return errors.Errorf("error writing file %s: %s", f.name, resp.String())
	}
	return nil
}

var _ fs.File = (*file)(nil)
var _ writefs.FileWrite = (*fileWrite)(nil)
