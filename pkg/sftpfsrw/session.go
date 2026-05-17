package sftpfsrw

import (
	"io/fs"
	"path/filepath"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func NewSession(conn *ssh.Client, sftpFS *sftpFSRW, i uint, logger zLogger.ZLogger) error {
	_logger := logger.With().Str("class", "sftpSession").Uint("session", i).Logger()
	_logger.Debug().Msgf("create sftp session %d", i)

	session, err := sftp.NewClient(conn)
	if err != nil {
		return errors.Wrap(err, "cannot create sftp client")
	}
	sftpFS.sftpSessions[i] = &sftpSession{
		Client: session,
		id:     i,
		sftpFS: sftpFS,
		logger: &_logger,
	}
	sftpFS.freeSessions <- i
	return nil
}

type sftpSession struct {
	*sftp.Client
	id     uint
	sftpFS *sftpFSRW
	logger zLogger.ZLogger
}

func (sess *sftpSession) Open(fullpath string) (fs.File, error) {
	sess.logger.Debug().Msgf("open '%s'", fullpath)
	fp, err := sess.Client.Open(fullpath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open '%s'", fullpath)
	}
	return &sftpFile{
		File: fp,
		sess: sess,
	}, nil
}

func (sess *sftpSession) Create(fullpath string) (writefs.FileWrite, error) {
	sess.logger.Debug().Msgf("create '%s'", fullpath)
	if err := sess.Client.MkdirAll(filepath.ToSlash(filepath.Dir(fullpath))); err != nil {
		return nil, errors.Wrapf(err, "cannot create directory '%s'", filepath.ToSlash(filepath.Dir(fullpath)))
	}
	fp, err := sess.Client.Create(fullpath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open '%s'", fullpath)
	}
	return &sftpFile{
		File: fp,
		sess: sess,
	}, nil
}
