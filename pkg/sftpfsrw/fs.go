package sftpfsrw

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"golang.org/x/crypto/ssh"
)

func NewFS(addr string, config *ssh.ClientConfig, baseDir string, numSessions uint, readOnly bool, logger zLogger.ZLogger) (*sftpFSRW, error) {
	_logger := logger.With().Str("class", "sftpFSRW").Logger()
	logger = &_logger
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot connect to '%s'", addr)
	}

	sftpFS := &sftpFSRW{
		readOnly:     readOnly,
		addr:         addr,
		user:         config.User,
		baseDir:      baseDir,
		sshClient:    client,
		sftpSessions: map[uint]*sftpSession{},
		freeSessions: make(chan uint, numSessions),
		logger:       logger,
	}

	for i := uint(0); i < numSessions; i++ {
		if err := NewSession(client, sftpFS, i, logger); err != nil {
			return nil, errors.Wrapf(err, "cannot create sftp session %d", i)
		}
	}

	return sftpFS, nil
}

type sftpFSRW struct {
	sshClient    *ssh.Client
	sftpSessions map[uint]*sftpSession
	addr         string
	user         string
	baseDir      string
	freeSessions chan uint
	logger       zLogger.ZLogger
	readOnly     bool
}

func (sftpFS *sftpFSRW) Equal(fsys fs.FS) bool {
	if sftpFS2, ok := fsys.(*sftpFSRW); ok {
		return sftpFS.addr == sftpFS2.addr && sftpFS.baseDir == sftpFS2.baseDir && sftpFS.user == sftpFS2.user
	}
	return false
}

func (sftpFS *sftpFSRW) Fullpath(name string) (string, error) {
	return filepath.ToSlash(filepath.Join(sftpFS.baseDir, name)), nil
}

func (sftpFS *sftpFSRW) Remove(path string) error {
	if sftpFS.readOnly {
		return errors.Errorf("read only filesystem")
	}
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return errors.Wrapf(err, "cannot get sftp session")
	}
	defer sftpFS.closeSession(sess)
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, path))
	if err := sess.Remove(fullpath); err != nil {
		var perr *fs.PathError
		if errors.As(err, &perr) {
			if strings.Contains(perr.Err.Error(), "file does not exist") {
				return errors.Append(fs.ErrNotExist, err)
			}
		}
		return errors.Wrapf(err, "cannot remove file %s", fullpath)
	}
	return nil
}

func (sftpFS *sftpFSRW) Rename(oldPath, newPath string) error {
	if sftpFS.readOnly {
		return errors.Errorf("read only filesystem")
	}
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return errors.Wrapf(err, "cannot get sftp session")
	}
	defer sftpFS.closeSession(sess)
	oldPath = filepath.ToSlash(filepath.Join(sftpFS.baseDir, oldPath))
	newPath = filepath.ToSlash(filepath.Join(sftpFS.baseDir, newPath))
	return sess.Rename(oldPath, newPath)
}

func (sftpFS *sftpFSRW) MkDir(path string) error {
	if sftpFS.readOnly {
		return errors.Errorf("read only filesystem")
	}
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return errors.Wrapf(err, "cannot get sftp session")
	}
	defer sftpFS.closeSession(sess)
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, path))
	return sess.Mkdir(fullpath)
}

func (sftpFS *sftpFSRW) Create(path string) (writefs.FileWrite, error) {
	if sftpFS.readOnly {
		return nil, errors.Errorf("read only filesystem")
	}
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get sftp session")
	}
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, path))
	fp, err := sess.Create(fullpath)
	if err != nil {
		sftpFS.closeSession(sess)
		return nil, errors.Wrapf(err, "cannot create '%s'", fullpath)
	}
	return fp, nil
}

func (sftpFS *sftpFSRW) Append(path string) (writefs.FileWrite, error) {
	if sftpFS.readOnly {
		return nil, errors.Errorf("read only filesystem")
	}
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get sftp session")
	}
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, path))
	fp, err := sess.OpenFile(fullpath, os.O_APPEND|os.O_WRONLY)
	if err != nil {
		sftpFS.closeSession(sess)
		return nil, errors.Wrapf(err, "cannot open '%s'", fullpath)
	}
	return fp, nil
}

func (sftpFS *sftpFSRW) ReadDir(name string) ([]fs.DirEntry, error) {
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get sftp session")
	}
	defer sftpFS.closeSession(sess)
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, name))
	dirs, err := sess.ReadDir(fullpath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read folder '%s'", fullpath)
	}
	ret := []fs.DirEntry{}
	for _, d := range dirs {
		fi := fs.FileInfoToDirEntry(d)
		if fi == nil {
			continue
		}
		ret = append(ret, fi)
	}
	return ret, err
}

func (sftpFS *sftpFSRW) ReadFile(name string) ([]byte, error) {
	fp, err := sftpFS.Open(name)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open '%s'", name)
	}
	defer fp.Close()
	data, err := io.ReadAll(fp)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read from '%s'", name)
	}
	return data, nil
}

func (sftpFS *sftpFSRW) Stat(name string) (fs.FileInfo, error) {
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get sftp session")
	}
	defer sftpFS.closeSession(sess)
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, name))
	fi, err := sess.Stat(fullpath)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot stat '%s'", fullpath)
	}
	return fi, nil
}

func (sftpFS *sftpFSRW) getSession(timeout time.Duration) (*sftpSession, error) {
	select {
	case i, ok := <-sftpFS.freeSessions:
		if !ok {
			return nil, errors.Errorf("error reading from channel")
		}
		return sftpFS.sftpSessions[i], nil
	case <-time.After(timeout):
		return nil, errors.Errorf("timeout reached")
	}
}

func (sftpFS *sftpFSRW) closeSession(sess *sftpSession) {
	sftpFS.freeSessions <- sess.id
}

func (sftpFS *sftpFSRW) String() string {
	return fmt.Sprintf("sftp://%s@%s/%s", sftpFS.user, sftpFS.addr, sftpFS.baseDir)
}

func (sftpFS *sftpFSRW) Open(name string) (fs.File, error) {
	sess, err := sftpFS.getSession(time.Second * 10)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get sftp session")
	}
	fullpath := filepath.ToSlash(filepath.Join(sftpFS.baseDir, name))
	fp, err := sess.Open(fullpath)
	if err != nil {
		sftpFS.closeSession(sess)
		return nil, errors.Wrapf(err, "cannot open '%s'", name)
	}
	return fp, nil
}

func (sftpFS *sftpFSRW) Close() error {
	var errs = []error{}
	close(sftpFS.freeSessions)

	for _, sess := range sftpFS.sftpSessions {
		if err := sess.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return nil
}

var (
	_ fmt.Stringer       = (*sftpFSRW)(nil)
	_ writefs.CreateFS   = (*sftpFSRW)(nil)
	_ writefs.AppendFS   = (*sftpFSRW)(nil)
	_ writefs.MkDirFS    = (*sftpFSRW)(nil)
	_ writefs.RenameFS   = (*sftpFSRW)(nil)
	_ writefs.RemoveFS   = (*sftpFSRW)(nil)
	_ writefs.CloseFS    = (*sftpFSRW)(nil)
	_ writefs.FullpathFS = (*sftpFSRW)(nil)
	_ writefs.EqualFS    = (*sftpFSRW)(nil)
	_ fs.FS              = (*sftpFSRW)(nil)
	_ fs.ReadDirFS       = (*sftpFSRW)(nil)
	_ fs.ReadFileFS      = (*sftpFSRW)(nil)
	_ fs.StatFS          = (*sftpFSRW)(nil)
)
