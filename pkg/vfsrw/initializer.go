package vfsrw

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/fs"
	"os"
	"time"

	"emperror.dev/errors"
	"github.com/je4/filesystem/v3/pkg/aferoFS"
	"github.com/je4/filesystem/v3/pkg/memFS"
	"github.com/je4/filesystem/v3/pkg/miniKVStoreFSRW"
	"github.com/je4/filesystem/v3/pkg/osfsrw"
	"github.com/je4/filesystem/v3/pkg/remotefs"
	"github.com/je4/filesystem/v3/pkg/s3fsrw"
	"github.com/je4/filesystem/v3/pkg/sftpfsrw"
	"github.com/je4/filesystem/v3/pkg/webFS"
	"github.com/je4/filesystem/v3/pkg/zipasfolder"
	"github.com/je4/utils/v2/pkg/zLogger"
	"go.ub.unibas.ch/cloud/certloader/v2/pkg/loader"
	resolver "go.ub.unibas.ch/cloud/miniresolverclient/pkg/miniresolverclient"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func (vfs *vFSRW) newRemote(name string, conf *Remote, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	clientTLS, clientLoader, err := loader.CreateClientLoader(conf.ClientTLS, logger)
	if err != nil {
		logger.Panic().Msgf("cannot create client loader: %v", err)
	}
	if len(conf.CAs) > 0 {
		caPool := x509.NewCertPool()
		for _, ca := range conf.CAs {
			caPool.AddCert(ca.Certificate)
		}
		clientTLS.RootCAs = caPool
	}
	if conf.InsecureSkipVerify {
		clientTLS.InsecureSkipVerify = true
	}
	rFS, err := remotefs.NewFS(clientTLS, conf.Address, conf.BaseDir, name, []io.Closer{clientLoader}, conf.JWTKey.String(), readOnly, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create new osfsrw")
	}
	return rFS, nil
}

func (vfs *vFSRW) newWeb(name string, cfg *Web, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	wFS, err := webFS.NewFS(cfg.BaseURI, cfg.Header, cfg.TLSInsecureSkipVerify, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create new webfs for '%s'", name)
	}
	return wFS, nil
}

func (vfs *vFSRW) newOS(name string, cfg *OS, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	rFS, err := osfsrw.NewFS(cfg.BaseDir, readOnly, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create new osfsrw")
	}
	if cfg.ZipAsFolderCache == 0 {
		return rFS, nil
	}
	zFS, err := zipasfolder.NewFS(rFS, int(cfg.ZipAsFolderCache), readOnly, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zipasfolder over '%v'", zFS)
	}
	return zFS, nil
}

func (vfs *vFSRW) newMiniKVStore(name string, store *MiniKVStore, readonly bool, logger zLogger.ZLogger) (fs.FS, []io.Closer, error) {
	toClose := []io.Closer{}
	if vfs.miniResolverClient == nil {
		var err error
		vfs.miniResolverClientTLS, vfs.miniResolverClientLoader, err = loader.CreateClientLoader(store.ClientTLS, logger)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "cannot create client loader for %s", name)
		}
		toClose = append(toClose, vfs.miniResolverClientLoader)

		logger.Info().Msgf("resolver address is %s", store.ResolverAddr)
		vfs.miniResolverClient, err = resolver.NewMiniresolverClient(
			store.ResolverAddr,
			nil,
			vfs.miniResolverClientTLS,
			nil,
			time.Duration(store.ResolverTimeout),
			time.Duration(store.ResolverNotFoundTimeout),
			logger)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "cannot create miniResolverClient for %s", name)
		}
		toClose = append(toClose, vfs.miniResolverClient)
	}
	fsys, err := miniKVStoreFSRW.NewFS(
		vfs.miniResolverClient,
		store.Domain,
		"minikvstore",
		store.Dir,
		nil,
		readonly,
		logger,
	)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot create miniKVStoreFSRW for %s", name)
	}
	return fsys, toClose, nil
}

func (vfs *vFSRW) newSFTP(name string, cfg *SFTP, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	if cfg.Sessions <= cfg.ZipAsFolderCache {
		return nil, errors.Errorf("sftp sessions (%v) must be larger than zipasfoldercache (%v)", cfg.Sessions, cfg.ZipAsFolderCache)
	}
	sConfig := &ssh.ClientConfig{
		User: string(cfg.User),
	}
	if len(cfg.PrivateKey) > 0 {
		var signers = []ssh.Signer{}
		for _, keyFile := range cfg.PrivateKey {
			pemBytes, err := os.ReadFile(keyFile)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot read '%s'", keyFile)
			}
			if cfg.Password != "" {
				signer, err := ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(cfg.Password))
				if err != nil {
					return nil, errors.Wrapf(err, "cannot parse and decrypt '%s'", keyFile)
				}
				signers = append(signers, signer)
			} else {
				signer, err := ssh.ParsePrivateKey(pemBytes)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot parse '%s'", keyFile)
				}
				signers = append(signers, signer)
			}
		}
		sConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signers...)}
	} else {
		// password login
		sConfig.Auth = []ssh.AuthMethod{ssh.Password(string(cfg.Password))}
	}
	if len(cfg.KnownHosts) == 0 {
		sConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		hkCallback, err := knownhosts.New(cfg.KnownHosts...)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot read known hosts %v", cfg.KnownHosts)
		}
		sConfig.HostKeyCallback = hkCallback
	}
	rFS, err := sftpfsrw.NewFS(string(cfg.Address), sConfig, cfg.BaseDir, cfg.Sessions, readOnly, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create sftpfsrw")
	}
	if cfg.ZipAsFolderCache == 0 {
		return rFS, nil
	}
	zFS, err := zipasfolder.NewFS(rFS, int(cfg.ZipAsFolderCache), readOnly, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zipasfolder over '%v'", zFS)
	}
	return zFS, nil
}

func (vfs *vFSRW) newS3(name string, cfg *S3, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	var tlsConfig *tls.Config
	switch cfg.CAPEM {
	case "ignore":
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	case "":
		// no tls
	default:
		tlsConfig = &tls.Config{RootCAs: x509.NewCertPool()}
		if ok := tlsConfig.RootCAs.AppendCertsFromPEM([]byte(cfg.CAPEM)); !ok {
			return nil, errors.New("cannot add root ca to CertPool")
		}
	}

	rFS, err := s3fsrw.NewFS(
		string(cfg.Endpoint),
		string(cfg.AccessKeyID),
		string(cfg.SecretAccessKey),
		string(cfg.Region),
		cfg.UseSSL,
		cfg.Debug,
		tlsConfig,
		cfg.DNSNetwork,
		cfg.DNSAddress,
		readOnly,
		logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create s3fsrw")
	}
	if cfg.ZipAsFolderCache == 0 {
		return rFS, nil
	}
	zFS, err := zipasfolder.NewFS(rFS, int(cfg.ZipAsFolderCache), readOnly, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create zipasfolder over '%v'", zFS)
	}
	return zFS, nil
}

func (vfs *vFSRW) newMemFS(name string, cfg *MemFS, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	return memFS.NewFS(logger)
}

func (vfs *vFSRW) newAfero(name string, cfg *Afero, readOnly bool, logger zLogger.ZLogger) (fs.FS, error) {
	createFunc := aferoFS.NewCreateFSFunc(logger)
	return createFunc(nil, cfg.BaseDir, readOnly)
}
