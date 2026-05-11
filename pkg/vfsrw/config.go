package vfsrw

import (
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/config"
)
import "go.ub.unibas.ch/cloud/certloader/v2/pkg/loader"
import certconfig "github.com/je4/trustutil/v2/pkg/config"

type SFTP struct {
	Address    config.EnvString `toml:"address"`
	KnownHosts []string         `toml:"knownhosts"`
	BaseDir    string           `toml:"basedir"`
	Sessions   uint             `toml:"sessions"`
	User       config.EnvString `toml:"user"`
	Password   config.EnvString `toml:"password"`
	PrivateKey []string         `toml:"privatekey"`
}

type Web struct {
	BaseURI               string              `toml:"baseuri"`
	Header                map[string][]string `toml:"header"`
	TLSInsecureSkipVerify bool                `toml:"tls_insecure_skip_verify"`
}
type OS struct {
	BaseDir string `toml:"basedir"`
}

type Remote struct {
	CAs                []certconfig.Certificate `toml:"ca"`
	Address            string                   `toml:"address"`
	ClientTLS          *loader.Config           `toml:"clienttls"`
	BaseDir            string                   `toml:"basedir"`
	JWTKey             config.EnvString         `toml:"jwtkey"`
	InsecureSkipVerify bool                     `toml:"insecureskipverify"`
}

type S3 struct {
	AccessKeyID     config.EnvString `toml:"accesskeyid"`
	SecretAccessKey config.EnvString `toml:"secretaccesskey"`
	Endpoint        config.EnvString `toml:"endpoint"`
	Region          config.EnvString `toml:"region"`
	UseSSL          bool             `toml:"usessl"`
	Debug           bool             `toml:"debug"`
	CAPEM           string           `toml:"capem"`
	BaseUrl         string           `toml:"baseurl"`
	DNSNetwork      string           `toml:"dnsnetwork"`
	DNSAddress      string           `toml:"dnsaddress"`
}

type MiniKVStore struct {
	ResolverAddr            string          `toml:"resolveraddr"`
	ResolverTimeout         config.Duration `toml:"resolvertimeout"`
	ResolverNotFoundTimeout config.Duration `toml:"resolvernotfoundtimeout"`
	Domain                  string          `toml:"domain"`
	ClientTLS               *loader.Config  `toml:"clienttls"`
	Dir                     string          `toml:"dir"`
}

type Afero struct {
	BaseDir string `toml:"basedir"`
}

type AESConfig struct {
	Enable       bool             `toml:"enable"`
	KeepassFile  config.EnvString `toml:"keepassfile"`
	KeepassEntry config.EnvString `toml:"keepassentry"`
	KeepassKey   config.EnvString `toml:"keepasskey"`
	IV           config.EnvString `toml:"iv"`
}

type ZipAsFolder struct {
	Enabled   bool                       `toml:"enabled"`
	Digests   []checksum.DigestAlgorithm `toml:"digests"`
	CacheSize uint                       `toml:"cachesize"`
	Compress  bool                       `toml:"compress"`
	ReadOnly  bool                       `toml:"readonly"`
	AES       *AESConfig                 `toml:"aes"`
}

type VFS struct {
	Name        string       `toml:"name"`
	Type        string       `toml:"type"`
	ReadOnly    bool         `toml:"readonly"`
	ZipAsFolder *ZipAsFolder `toml:"zipasfolder"`
	S3          *S3          `toml:"s3,omitempty"`
	OS          *OS          `toml:"os,omitempty"`
	SFTP        *SFTP        `toml:"sftp,omitempty"`
	Remote      *Remote      `toml:"remote,omitempty"`
	MiniKVStore *MiniKVStore `toml:"minikvstore,omitempty"`
	Web         *Web         `toml:"web,omitempty"`
	Afero       *Afero       `toml:"afero,omitempty"`
}

type Config map[string]*VFS
