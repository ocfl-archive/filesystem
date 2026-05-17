package zipfsrw

import (
	"io/fs"
	"strings"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
)

func NewCreateFSFunc(noCompression bool, logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, zipFile string, readOnly bool) (fs.FS, error) {
		parts := strings.Split(zipFile, "/")
		if len(parts) < 2 {
			return nil, errors.Errorf("invalid zip path: %s", zipFile)
		}
		baseFS, err := f.Get(strings.Join(parts[:len(parts)-1], "/"), readOnly)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get base filesystem for '%s'", zipFile)
		}
		zipFS, err := NewFSFile(baseFS, parts[len(parts)-1], noCompression, readOnly, logger)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create zip filesystem for '%s'", zipFile)
		}
		return zipFS, nil
	}
}

func NewCreateFSChecksumFunc(noCompression bool, algs []checksum.DigestAlgorithm, logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, zipFile string, readOnly bool) (fs.FS, error) {
		parts := strings.Split(zipFile, "/")
		if len(parts) < 2 {
			return nil, errors.Errorf("invalid zip path: %s", zipFile)
		}
		baseFS, err := f.Get(strings.Join(parts[:len(parts)-1], "/"), readOnly)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get base filesystem for '%s'", zipFile)
		}
		zipFS, err := NewFSFileChecksums(baseFS, parts[len(parts)-1], noCompression, algs, readOnly, logger)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create zip filesystem for '%s'", zipFile)
		}
		return zipFS, nil
	}
}

func NewCreateFSEncryptedChecksumFunc(noCompression bool, algs []checksum.DigestAlgorithm, keyUri string, logger zLogger.ZLogger) writefs.CreateFSFunc {
	return func(f writefs.IFactory, zipFile string, readOnly bool) (fs.FS, error) {
		parts := strings.Split(zipFile, "/")
		if len(parts) < 2 {
			return nil, errors.Errorf("invalid zip path: %s", zipFile)
		}
		baseFS, err := f.Get(strings.Join(parts[:len(parts)-1], "/"), readOnly)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get base filesystem for '%s'", zipFile)
		}

		zipFS, err := NewFSFileEncryptedChecksums(baseFS, parts[len(parts)-1], noCompression, algs, keyUri, logger)
		//		zipReader, err := NewFSFileChecksums(baseFS, parts[len(parts)-1], noCompression, algs)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot create zip filesystem for '%s'", zipFile)
		}
		return zipFS, nil
	}
}
