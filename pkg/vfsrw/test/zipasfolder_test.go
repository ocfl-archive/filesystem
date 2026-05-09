package vfsrw_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sftp_test_server "github.com/JuniorGuerra/sftp_test_server"
	"github.com/je4/filesystem/v4/pkg/vfsrw"
	"github.com/je4/filesystem/v4/pkg/writefs"
	"github.com/je4/utils/v2/pkg/config"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/rs/zerolog"
)

func createTestZip() []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("test.txt")
	f.Write([]byte("hello zip test.zip"))
	f2, _ := zw.Create("sub/subtest.txt")
	f2.Write([]byte("hello sub zip"))
	f3, _ := zw.Create("sub/deep/deeptest.txt")
	f3.Write([]byte("hello deep zip"))
	zw.Close()
	return buf.Bytes()
}

func createCustomTestZip(name, content string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	f, _ := zw.Create(name)
	f.Write([]byte(content))
	zw.Close()
	return buf.Bytes()
}

var (
	zipData1 = createCustomTestZip("test.txt", "hello zip test1.zip")
	zipData2 = createCustomTestZip("test.txt", "hello zip test2.zip")
	zipData3 = createCustomTestZip("test.txt", "hello zip test3.zip")
)

func runZipAsFolderTest(t *testing.T, vfs vfsrw.VFSRW, fsName string) {
	zipData := createTestZip()
	var zipPath string
	if fsName == "s3" {
		zipPath = fmt.Sprintf("vfs://%s/testbucket/test.zip", fsName)
	} else if fsName == "web" {
		zipPath = fmt.Sprintf("vfs://%s/test.zip", fsName)
	} else {
		zipPath = fmt.Sprintf("vfs://%s/test.zip", fsName)
	}

	if fsName != "web" {
		if _, err := writefs.WriteFile(vfs, zipPath, zipData); err != nil {
			t.Fatalf("failed to write zip file: %v", err)
		}
	}

	// Test access into zip
	testFile := zipPath + "/test.txt"
	data, err := vfs.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file in zip: %v", err)
	}
	if string(data) != "hello zip test.zip" {
		t.Fatalf("expected 'hello zip test.zip', got '%s'", string(data))
	}

	deepFile := zipPath + "/sub/deep/deeptest.txt"
	data, err = vfs.ReadFile(deepFile)
	if err != nil {
		t.Fatalf("failed to read deep file in zip: %v", err)
	}
	if string(data) != "hello deep zip" {
		t.Fatalf("expected 'hello deep zip', got '%s'", string(data))
	}

	// Test ReadDir
	t.Logf("ReadDir %s", zipPath)
	entries, err := vfs.ReadDir(zipPath)
	if err != nil {
		t.Fatalf("ReadDir on zip failed: %v", err)
	}
	foundSub := false
	for _, e := range entries {
		if e.Name() == "sub" {
			foundSub = true
			if !e.IsDir() {
				t.Errorf("expected 'sub' to be a directory")
			}
		}
	}
	if !foundSub {
		t.Errorf("'sub' directory not found in zip")
	}

	// Test Stat on zip itself as folder
	t.Logf("stating %s", zipPath)
	fi, err := vfs.Stat(zipPath)
	if err != nil {
		// Try with trailing slash
		t.Logf("stat on %s failed: %v, trying with trailing slash", zipPath, err)
		fi, err = vfs.Stat(zipPath + "/")
	}
	if err != nil {
		t.Fatalf("stat on zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected zip to be treated as directory")
	}
}

func setupSFTPServer(t *testing.T, port int) (server *sftp_test_server.SFTPServer, tempDir string, user, password string) {
	tempDir, err := os.MkdirTemp("", "sftp_test_server")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	user = "testuser"
	password = "testpass"
	server, err = sftp_test_server.NewSFTPServerLocal(user, password, port, tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to create sftp server: %v", err)
	}

	go func() {
		if err := server.Start(); err != nil {
			t.Logf("sftp server stopped: %v", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(200 * time.Millisecond)
	return server, tempDir, user, password
}

func setupOSTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "vfs_os_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return filepath.ToSlash(tempDir)
}

func setupS3Server(t *testing.T) (endpoint string, bucket string, accessKey string, secretKey string, closer func()) {
	backend := s3mem.New()
	fakes3 := gofakes3.New(backend)
	srv := httptest.NewServer(fakes3.Server())

	u, _ := url.Parse(srv.URL)
	endpoint = u.Host
	t.Logf("S3 Server started at %s (endpoint: %s)", srv.URL, endpoint)

	accessKey = "access"
	secretKey = "secret"
	bucket = "testbucket"

	if err := backend.CreateBucket(bucket); err != nil {
		srv.Close()
		t.Fatalf("failed to create bucket: %v", err)
	}

	return endpoint, bucket, accessKey, secretKey, srv.Close
}

func TestZipAsFolder_Afero(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			Afero:            &vfsrw.Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	runZipAsFolderTest(t, vfs, "mem")
}

func TestZipAsFolder_OS(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	tempDir := setupOSTempDir(t)
	defer os.RemoveAll(tempDir)

	t.Logf("Base Dir: %s", tempDir)
	cfg := vfsrw.Config{
		"os": &vfsrw.VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			OS:               &vfsrw.OS{BaseDir: tempDir},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	runZipAsFolderTest(t, vfs, "os")
}

func TestZipAsFolder_SFTP(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	port := 22222
	server, tempDir, user, password := setupSFTPServer(t, port)
	defer os.RemoveAll(tempDir)
	defer server.Stop()

	cfg := vfsrw.Config{
		"sftp": &vfsrw.VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	runZipAsFolderTest(t, vfs, "sftp")
}

func TestZipAsFolder_S3(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	endpoint, _, accessKey, secretKey, closer := setupS3Server(t)
	defer closer()

	cfg := vfsrw.Config{
		"s3": &vfsrw.VFS{
			Name:             "s3",
			Type:             "s3",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			S3: &vfsrw.S3{
				Endpoint:        config.EnvString(endpoint),
				AccessKeyID:     config.EnvString(accessKey),
				SecretAccessKey: config.EnvString(secretKey),
				Region:          "us-east-1",
				UseSSL:          false,
				CAPEM:           "ignore",
			},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	runZipAsFolderTest(t, vfs, "s3")
}

func TestZipAsFolder_WebFS(t *testing.T) {
	zipData := createTestZip()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "test.zip", time.Now(), bytes.NewReader(zipData))
	}))
	defer ts.Close()

	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger
	vfs, err := vfsrw.NewFS(vfsrw.Config{
		"web": &vfsrw.VFS{
			Name:             "web",
			Type:             "web",
			ReadOnly:         true,
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			Web: &vfsrw.Web{
				BaseURI: ts.URL + "/%%PATH%%",
			},
		},
	}, _logger)
	if err != nil {
		t.Fatal(err)
	}

	runZipAsFolderTest(t, vfs, "web")
}

func TestZipAsFolder_ReadDir(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	// 1. SFTP Setup
	port := 22223
	server, sftpTempDir, user, password := setupSFTPServer(t, port)
	defer os.RemoveAll(sftpTempDir)
	defer server.Stop()

	// 2. OS Setup
	osTempDir := setupOSTempDir(t)
	defer os.RemoveAll(osTempDir)

	// 3. S3 Setup
	s3Endpoint, _, s3AccessKey, s3SecretKey, s3Closer := setupS3Server(t)
	defer s3Closer()

	// 4. Web Setup
	zipData := createTestZip()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just serve the same zip for any request
		http.ServeContent(w, r, r.URL.Path, time.Now(), bytes.NewReader(zipData))
	}))
	defer ts.Close()

	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			Afero:            &vfsrw.Afero{BaseDir: "mem://"},
		},
		"os": &vfsrw.VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			OS:               &vfsrw.OS{BaseDir: osTempDir},
		},
		"sftp": &vfsrw.VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
		"s3": &vfsrw.VFS{
			Name:             "s3",
			Type:             "s3",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			S3: &vfsrw.S3{
				Endpoint:        config.EnvString(s3Endpoint),
				AccessKeyID:     config.EnvString(s3AccessKey),
				SecretAccessKey: config.EnvString(s3SecretKey),
				Region:          "us-east-1",
				UseSSL:          false,
				CAPEM:           "ignore",
			},
		},
		"web": &vfsrw.VFS{
			Name:             "web",
			Type:             "web",
			ZipAsFolder:      true,
			ZipAsFolderCache: 10,
			Web: &vfsrw.Web{
				BaseURI: ts.URL + "/%%PATH%%",
			},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	backends := []string{"mem", "os", "sftp", "s3", "web"}

	for _, be := range backends {
		t.Run("Backend_"+be, func(t *testing.T) {
			var basePath string
			if be == "s3" {
				basePath = "vfs://s3/testbucket/"
			} else {
				basePath = fmt.Sprintf("vfs://%s/", be)
			}
			zipPath := basePath + "test.zip"

			if writefs.IsWriteable(vfs, zipPath) {
				if _, err := writefs.WriteFile(vfs, zipPath, zipData); err != nil {
					t.Fatalf("[%s] failed to write zip file: %v", be, err)
				}
			}

			if be == "web" {
				// WebFS does not support ReadDir on the base directory generally,
				// as it doesn't know which files exist on the server.
				t.Logf("[%s] Skipping ReadDir on base directory for WebFS", be)
			} else {
				res, err := vfs.ReadDir(basePath)
				if err != nil {
					t.Fatalf("[%s] ReadDir on %s failed: %v", be, basePath, err)
				}
				foundZip := false
				for _, e := range res {
					t.Logf("[%s] Entry in root: %s (IsDir: %v)", be, e.Name(), e.IsDir())
					if e.Name() == "test.zip" {
						foundZip = true
						if !e.IsDir() {
							t.Errorf("[%s] expected test.zip to be a directory in root listing", be)
						}
					}
				}
				if !foundZip {
					t.Errorf("[%s] test.zip not found in root directory listing", be)
				}
			}

			// Nun IN das Zip schauen
			entries, err := vfs.ReadDir(zipPath)
			if err != nil {
				t.Fatalf("[%s] ReadDir on zip failed: %v", be, err)
			}
			foundSub := false
			for _, e := range entries {
				if e.Name() == "sub" {
					foundSub = true
					if !e.IsDir() {
						t.Errorf("[%s] expected sub to be a directory", be)
					}
				}
			}
			if !foundSub {
				t.Errorf("[%s] sub directory not found in zip root", be)
			}

			// Noch tiefer schauen
			entries, err = vfs.ReadDir(zipPath + "/sub")
			if err != nil {
				t.Fatalf("[%s] ReadDir on zip/sub failed: %v", be, err)
			}
			foundDeep := false
			for _, e := range entries {
				if e.Name() == "deep" {
					foundDeep = true
					if !e.IsDir() {
						t.Errorf("[%s] expected deep to be a directory", be)
					}
				}
			}
			if !foundDeep {
				t.Errorf("[%s] deep directory not found in zip/sub", be)
			}

			// Wir akzeptieren hier eine leere Liste, wenn keine expliziten Verzeichnisse da sind,
			// aber Stat muss funktionieren.
			fi, err := vfs.Stat(zipPath + "/sub/deep/deeptest.txt")
			if err != nil {
				t.Errorf("[%s] Stat on deeptest.txt in zip failed: %v", be, err)
			} else {
				t.Logf("[%s] Stat on deeptest.txt in zip succeeded: %s, %d", be, fi.Name(), fi.Size())
			}
		})
	}
}

func TestZipAsFolder_CacheLimit(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	// 1. SFTP Setup
	port := 22224
	server, sftpTempDir, user, password := setupSFTPServer(t, port)
	defer os.RemoveAll(sftpTempDir)
	defer server.Stop()

	// 2. OS Setup
	osTempDir := setupOSTempDir(t)
	defer os.RemoveAll(osTempDir)

	// 3. S3 Setup
	s3Endpoint, _, s3AccessKey, s3SecretKey, s3Closer := setupS3Server(t)
	defer s3Closer()

	// 4. Web Setup
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		var data []byte
		switch name {
		case "test1.zip":
			data = zipData1
		case "test2.zip":
			data = zipData2
		case "test3.zip":
			data = zipData3
		default:
			data = createCustomTestZip("test.txt", fmt.Sprintf("hello zip %s", name))
		}
		// Just serve the same zip for any request
		http.ServeContent(w, r, r.URL.Path, time.Now(), bytes.NewReader(data))
	}))
	defer ts.Close()

	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolder:      true,
			ZipAsFolderCache: 2,
			Afero:            &vfsrw.Afero{BaseDir: "mem://"},
		},
		"os": &vfsrw.VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolder:      true,
			ZipAsFolderCache: 2,
			OS:               &vfsrw.OS{BaseDir: osTempDir},
		},
		"sftp": &vfsrw.VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolder:      true,
			ZipAsFolderCache: 2,
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
		"s3": &vfsrw.VFS{
			Name:             "s3",
			Type:             "s3",
			ZipAsFolder:      true,
			ZipAsFolderCache: 2,
			S3: &vfsrw.S3{
				Endpoint:        config.EnvString(s3Endpoint),
				AccessKeyID:     config.EnvString(s3AccessKey),
				SecretAccessKey: config.EnvString(s3SecretKey),
				Region:          "us-east-1",
				UseSSL:          false,
				CAPEM:           "ignore",
			},
		},
		"web": &vfsrw.VFS{
			Name:             "web",
			Type:             "web",
			ZipAsFolder:      true,
			ZipAsFolderCache: 2,
			Web: &vfsrw.Web{
				BaseURI: ts.URL + "/%%PATH%%",
			},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	backends := []string{"mem", "os", "sftp", "s3", "web"}

	for _, be := range backends {
		t.Run("Backend_"+be, func(t *testing.T) {
			// Write 3 zip files for writeable backends
			zipPath1Tmp := fmt.Sprintf("vfs://%s/test1.zip", be)
			if be == "s3" {
				zipPath1Tmp = fmt.Sprintf("vfs://%s/testbucket/test1.zip", be)
			}
			if writefs.IsWriteable(vfs, zipPath1Tmp) {
				for i := 1; i <= 3; i++ {
					var zp string
					if be == "s3" {
						zp = fmt.Sprintf("vfs://%s/testbucket/test%d.zip", be, i)
					} else {
						zp = fmt.Sprintf("vfs://%s/test%d.zip", be, i)
					}
					var data []byte
					switch i {
					case 1:
						data = zipData1
					case 2:
						data = zipData2
					case 3:
						data = zipData3
					default:
						data = createCustomTestZip("test.txt", fmt.Sprintf("hello zip test%d.zip", i))
					}
					if _, err := writefs.WriteFile(vfs, zp, data); err != nil {
						t.Fatalf("failed to write zip file %s: %v", zp, err)
					}
				}
			}

			var zipPath1, zipPath2, zipPath3 string
			if be == "web" {
				zipPath1 = "vfs://web/test1.zip"
				zipPath2 = "vfs://web/test2.zip"
				zipPath3 = "vfs://web/test3.zip"
			} else if be == "s3" {
				zipPath1 = fmt.Sprintf("vfs://%s/testbucket/test1.zip", be)
				zipPath2 = fmt.Sprintf("vfs://%s/testbucket/test2.zip", be)
				zipPath3 = fmt.Sprintf("vfs://%s/testbucket/test3.zip", be)
			} else {
				zipPath1 = fmt.Sprintf("vfs://%s/test1.zip", be)
				zipPath2 = fmt.Sprintf("vfs://%s/test2.zip", be)
				zipPath3 = fmt.Sprintf("vfs://%s/test3.zip", be)
			}

			if writefs.IsWriteable(vfs, zipPath1) {
				if _, err := writefs.WriteFile(vfs, zipPath1, zipData1); err != nil {
					t.Fatalf("failed to write zip file 1: %v", err)
				}
				if _, err := writefs.WriteFile(vfs, zipPath2, zipData2); err != nil {
					t.Fatalf("failed to write zip file 2: %v", err)
				}
				if _, err := writefs.WriteFile(vfs, zipPath3, zipData3); err != nil {
					t.Fatalf("failed to write zip file 3: %v", err)
				}
			}

			// Zugriff auf test1.zip -> Geladen (Cache: [1])
			t.Logf("[%s] Accessing test1.zip", be)
			if _, err := vfs.ReadFile(zipPath1 + "/test.txt"); err != nil {
				t.Fatalf("failed to read test1.zip: %v", err)
			}

			// Zugriff auf test2.zip -> Geladen (Cache: [1, 2])
			t.Logf("[%s] Accessing test2.zip", be)
			if _, err := vfs.ReadFile(zipPath2 + "/test.txt"); err != nil {
				t.Fatalf("failed to read test2.zip: %v", err)
			}

			// Zugriff auf test3.zip -> Geladen, test1.zip sollte verdrängt werden (Cache: [2, 3])
			t.Logf("[%s] Accessing test3.zip (should evict test1.zip)", be)
			if _, err := vfs.ReadFile(zipPath3 + "/test.txt"); err != nil {
				t.Fatalf("failed to read test3.zip: %v", err)
			}

			// Erneuter Zugriff auf test1.zip -> Sollte neu geladen werden (Cache: [3, 1], test2.zip verdrängt)
			t.Logf("[%s] Accessing test1.zip again (should evict test2.zip)", be)
			if _, err := vfs.ReadFile(zipPath1 + "/test.txt"); err != nil {
				t.Fatalf("failed to read test1.zip again: %v", err)
			}
		})
	}
}

func TestZipAsFolder_Stat(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolder:      true,
			ZipAsFolderCache: 2,
			Afero:            &vfsrw.Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	zipData := createTestZip()
	if _, err := writefs.WriteFile(vfs, "vfs://mem/test.zip", zipData); err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	// 1. Stat auf das Zip selbst
	fi, err := vfs.Stat("vfs://mem/test.zip")
	if err != nil {
		t.Fatalf("Stat on test.zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected test.zip to be a directory")
	}

	// 2. Stat auf Datei im Zip
	fi, err = vfs.Stat("vfs://mem/test.zip/test.txt")
	if err != nil {
		t.Fatalf("Stat on test.txt in zip failed: %v", err)
	}
	if fi.IsDir() {
		t.Errorf("expected test.txt to be a file")
	}
	if fi.Name() != "test.txt" {
		t.Errorf("expected name 'test.txt', got '%s'", fi.Name())
	}

	// 3. Stat auf Verzeichnis im Zip
	fi, err = vfs.Stat("vfs://mem/test.zip/sub")
	if err != nil {
		t.Fatalf("Stat on sub in zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected sub to be a directory")
	}

	// 4. Stat auf tiefes Verzeichnis im Zip
	fi, err = vfs.Stat("vfs://mem/test.zip/sub/deep")
	if err != nil {
		t.Fatalf("Stat on sub/deep in zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected sub/deep to be a directory")
	}

	// 5. Stat auf Datei in tiefem Verzeichnis
	fi, err = vfs.Stat("vfs://mem/test.zip/sub/deep/deeptest.txt")
	if err != nil {
		t.Fatalf("Stat on deeptest.txt in zip failed: %v", err)
	}
	if fi.IsDir() {
		t.Errorf("expected deeptest.txt to be a file")
	}

	// 6. Stat auf nicht existierende Datei im Zip
	_, err = vfs.Stat("vfs://mem/test.zip/nonexistent.txt")
	if err == nil {
		t.Errorf("expected error for nonexistent file in zip")
	}
}

func TestZipAsFolder_Concurrency(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	// 1. SFTP Setup
	port := 22223 // Anders als in anderen Tests
	server, sftpTempDir, user, password := setupSFTPServer(t, port)
	defer os.RemoveAll(sftpTempDir)
	defer server.Stop()

	// 2. OS Setup
	osTempDir := setupOSTempDir(t)
	defer os.RemoveAll(osTempDir)

	// 3. S3 Setup
	s3Endpoint, _, s3AccessKey, s3SecretKey, s3Closer := setupS3Server(t)
	defer s3Closer()

	// 4. Web Setup
	zipData := createCustomTestZip("test.txt", "hello world")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, r.URL.Path, time.Now(), bytes.NewReader(zipData))
	}))
	defer ts.Close()

	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolder:      true,
			ZipAsFolderCache: 20,
			Afero:            &vfsrw.Afero{BaseDir: "mem://"},
		},
		"os": &vfsrw.VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolder:      true,
			ZipAsFolderCache: 20,
			OS:               &vfsrw.OS{BaseDir: osTempDir},
		},
		"sftp": &vfsrw.VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolder:      true,
			ZipAsFolderCache: 20,
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 5,
			},
		},
		"s3": &vfsrw.VFS{
			Name:             "s3",
			Type:             "s3",
			ZipAsFolder:      true,
			ZipAsFolderCache: 20,
			S3: &vfsrw.S3{
				Endpoint:        config.EnvString(s3Endpoint),
				AccessKeyID:     config.EnvString(s3AccessKey),
				SecretAccessKey: config.EnvString(s3SecretKey),
				Region:          "us-east-1",
				UseSSL:          false,
				CAPEM:           "ignore",
			},
		},
		"web": &vfsrw.VFS{
			Name:             "web",
			Type:             "web",
			ZipAsFolder:      true,
			ZipAsFolderCache: 20,
			Web: &vfsrw.Web{
				BaseURI: ts.URL + "/%%PATH%%",
			},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	backends := []string{"mem", "os", "sftp", "s3", "web"}
	numZips := 5
	zipData = createCustomTestZip("test.txt", "hello world")

	for _, be := range backends {
		basePath := fmt.Sprintf("vfs://%s/", be)
		if be == "s3" {
			basePath = fmt.Sprintf("vfs://%s/testbucket/", be)
		}
		if !writefs.IsWriteable(vfs, basePath) {
			continue // Skip read-only backends in this test setup
		}
		for i := range numZips {
			var name string
			if be == "s3" {
				name = fmt.Sprintf("vfs://%s/testbucket/test%d.zip", be, i)
			} else {
				name = fmt.Sprintf("vfs://%s/test%d.zip", be, i)
			}
			if _, err := writefs.WriteFile(vfs, name, zipData); err != nil {
				t.Fatalf("failed to write zip file %s: %v", name, err)
			}
		}
	}

	numWorkers := 30
	numIterations := 15
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for j := range numIterations {
				beIdx := (workerID + j) % len(backends)
				backend := backends[beIdx]
				zipID := (workerID + j) % numZips

				var path string
				if backend == "web" {
					path = fmt.Sprintf("vfs://web/test%d.zip/test.txt", zipID)
				} else if backend == "s3" {
					path = fmt.Sprintf("vfs://%s/testbucket/test%d.zip/test.txt", backend, zipID)
				} else {
					path = fmt.Sprintf("vfs://%s/test%d.zip/test.txt", backend, zipID)
				}

				// ReadFile
				data, err := vfs.ReadFile(path)
				if err != nil {
					t.Errorf("Worker %d: ReadFile failed for %s: %v", workerID, path, err)
					return
				}
				if string(data) != "hello world" {
					t.Errorf("Worker %d: unexpected data from %s: %s", workerID, path, string(data))
					return
				}

				// Stat
				_, err = vfs.Stat(path)
				if err != nil {
					t.Errorf("Worker %d: Stat failed for %s: %v", workerID, path, err)
					return
				}

				// ReadDir
				var dirPath string
				if backend == "s3" {
					dirPath = fmt.Sprintf("vfs://%s/testbucket/test%d.zip", backend, zipID)
				} else {
					dirPath = fmt.Sprintf("vfs://%s/test%d.zip", backend, zipID)
				}
				_, err = vfs.ReadDir(dirPath)
				if err != nil {
					t.Errorf("Worker %d: ReadDir failed for %s: %v", workerID, dirPath, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}
