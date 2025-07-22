package vfsrw

import "github.com/je4/utils/v2/pkg/config"
import "go.ub.unibas.ch/cloud/certloader/v2/pkg/loader"
import certconfig "github.com/je4/trustutil/v2/pkg/config"

type SFTP struct {
	Address          config.EnvString
	KnownHosts       []string
	BaseDir          string
	Sessions         uint
	User             config.EnvString
	Password         config.EnvString
	PrivateKey       []string
	ZipAsFolderCache uint
}

type OS struct {
	BaseDir          string
	ZipAsFolderCache uint
}

type Remote struct {
	CAs       []certconfig.Certificate `toml:"ca"`
	Address   string
	ClientTLS *loader.Config
	BaseDir   string
	JWTKey    config.EnvString
}

type S3 struct {
	AccessKeyID      config.EnvString
	SecretAccessKey  config.EnvString
	Endpoint         config.EnvString
	Region           config.EnvString
	UseSSL           bool
	Debug            bool
	CAPEM            string
	BaseUrl          string
	ZipAsFolderCache uint
	DNSNetwork       string
	DNSAddress       string
}

type MiniKVStore struct {
	ResolverAddr            string          `toml:"resolveraddr"`
	ResolverTimeout         config.Duration `toml:"resolvertimeout"`
	ResolverNotFoundTimeout config.Duration `toml:"resolvernotfoundtimeout"`
	Domain                  string          `toml:"domain"`
	ClientTLS               *loader.Config  `toml:"clienttls"`
	Dir                     string          `toml:"dir"`
}

type VFS struct {
	Name        string       `toml:"name"`
	Type        string       `toml:"type"`
	ReadOnly    bool         `toml:"readonly"`
	S3          *S3          `toml:"s3,omitempty"`
	OS          *OS          `toml:"os,omitempty"`
	SFTP        *SFTP        `toml:"sftp,omitempty"`
	Remote      *Remote      `toml:"remote,omitempty"`
	MiniKVStore *MiniKVStore `toml:"minikvstore,omitempty"`
}

type Config map[string]*VFS
