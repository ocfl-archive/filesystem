package vfsrw_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	sftp_test_server "github.com/JuniorGuerra/sftp_test_server"
	"github.com/je4/utils/v2/pkg/checksum"
	"github.com/je4/utils/v2/pkg/config"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
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
	if fsName == "s3" {
		// t.Skip("Skipping S3 due to eventual consistency (file not visible immediately after write)")
		//time.Sleep(500 * time.Millisecond) // Give S3 a moment to find the bucket/files
	}
	if fsName == "sftp" {
		// SFTP is generally OK for simple sequential tests
	}
	var zipPath string
	if fsName == "s3" {
		zipPath = fmt.Sprintf("vfs:/%s/testbucket/test.zip", fsName)
	} else if fsName == "web" {
		zipPath = fmt.Sprintf("vfs:/%s/test.zip", fsName)
	} else {
		zipPath = fmt.Sprintf("vfs:/%s/test.zip", fsName)
	}

	if fsName != "web" {
		subFS, err := vfs.SubCreate(zipPath)
		if err != nil {
			t.Fatalf("failed to create zip via vfs.Sub: %v", err)
		}
		if _, err := writefs.WriteFile(subFS, "test.txt", []byte("hello zip test.zip")); err != nil {
			t.Fatalf("failed to write test.txt to zip: %v", err)
		}
		if _, err := writefs.WriteFile(subFS, "sub/subtest.txt", []byte("hello sub zip")); err != nil {
			t.Fatalf("failed to write sub/subtest.txt to zip: %v", err)
		}
		if _, err := writefs.WriteFile(subFS, "sub/deep/deeptest.txt", []byte("hello deep zip")); err != nil {
			t.Fatalf("failed to write sub/deep/deeptest.txt to zip: %v", err)
		}
		if closer, ok := subFS.(io.Closer); ok {
			closer.Close()
		}
		//time.Sleep(1 * time.Second) // Give S3 a moment to be consistent

		// Check for checksum files
		for _, alg := range []checksum.DigestAlgorithm{checksum.DigestSHA512} {
			checksumFile := zipPath + "." + string(alg)
			if _, err := vfs.Stat(checksumFile); err != nil {
				t.Errorf("expected checksum file '%s' to exist, got error: %v", checksumFile, err)
			} else {
				t.Logf("checksum file '%s' exists", checksumFile)
				if fsName == "s3" {
					t.Logf("[%s] Skipping checksum content verification for S3 due to GoFakeS3 chunked encoding issues", fsName)
					continue
				}
				data, err := vfs.ReadFile(checksumFile)
				if err != nil {
					t.Errorf("failed to read checksum file '%s': %v", checksumFile, err)
				} else {
					t.Logf("checksum file '%s' content: %s", checksumFile, string(data))
					dataStr := string(data)
					// Handle S3 Chunked Encoding metadata if present
					if strings.Contains(dataStr, ";chunk-signature=") {
						parts := strings.Split(dataStr, "\n")
						if len(parts) > 1 {
							dataStr = strings.Join(parts[1:], "\n")
						}
					}
					parts := strings.Fields(dataStr)
					if len(parts) < 1 {
						t.Errorf("invalid checksum file format: %s", string(data))
					} else {
						expectedChecksum := parts[0]
						zipData, err := vfs.ReadFile(zipPath)
						if err != nil {
							t.Errorf("failed to read zip file '%s' for checksum verification: %v", zipPath, err)
						} else {
							csWriter, err := checksum.NewChecksumWriter([]checksum.DigestAlgorithm{alg}, io.Discard)
							if err != nil {
								t.Errorf("failed to create checksum writer: %v", err)
							} else {
								if _, err := io.Copy(csWriter, bytes.NewReader(zipData)); err != nil {
									t.Errorf("failed to write zip data to checksum writer: %v", err)
								} else {
									if err := csWriter.Close(); err != nil {
										t.Errorf("failed to close checksum writer: %v", err)
									} else {
										actualChecksums, err := csWriter.GetChecksums()
										if err != nil {
											t.Errorf("failed to get checksums: %v", err)
										} else {
											actualChecksum := actualChecksums[alg]
											if actualChecksum != expectedChecksum {
												t.Errorf("checksum mismatch for '%s': expected %s, got %s", zipPath, expectedChecksum, actualChecksum)
											} else {
												t.Logf("checksum for '%s' is correct: %s", zipPath, actualChecksum)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	} else {
		// webFS is read-only in this test, so we still use the pre-generated data for the server
		zipData := createTestZip()
		// (The httptest.Server in TestZipAsFolder_WebFS uses zipData)
		// We leave this as it is because webFS is a mock server in the test.
		_ = zipData
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
		if e.Name() == zipPath+"/sub" {
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
	//time.Sleep(200 * time.Millisecond)
	return server, tempDir, user, password
}

func setupOSTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "vfs_os_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return filepath.ToSlash(tempDir)
}

func setupS3Server(t *testing.T) (endpoint string, bucket string, accessKey string, secretKey string, fakes3 *gofakes3.GoFakeS3, closer func()) {
	backend := s3mem.New()
	fakes3 = gofakes3.New(backend)
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

	return endpoint, bucket, accessKey, secretKey, fakes3, srv.Close
}

func TestZipAsFolder_Afero(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
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
			Name:        "os",
			Type:        "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			OS:          &vfsrw.OS{BaseDir: tempDir},
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
			Name:        "sftp",
			Type:        "sftp",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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

	endpoint, _, accessKey, secretKey, fakes3, closer := setupS3Server(t)
	_ = fakes3
	defer closer()

	cfg := vfsrw.Config{
		"s3": &vfsrw.VFS{
			Name:        "s3",
			Type:        "s3",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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

	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	vfs, err := vfsrw.NewFS(vfsrw.Config{
		"web": &vfsrw.VFS{
			Name:        "web",
			Type:        "web",
			ReadOnly:    true,
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
	s3Endpoint, _, s3AccessKey, s3SecretKey, _, s3Closer := setupS3Server(t)
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
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
		"os": &vfsrw.VFS{
			Name:        "os",
			Type:        "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			OS:          &vfsrw.OS{BaseDir: osTempDir},
		},
		"sftp": &vfsrw.VFS{
			Name:        "sftp",
			Type:        "sftp",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
		"s3": &vfsrw.VFS{
			Name:        "s3",
			Type:        "s3",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
			Name:        "web",
			Type:        "web",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
			if be == "s3" {
				// t.Skip("Skipping S3 due to eventual consistency issues")
				vfs.MkDir("vfs://s3/testbucket")
				//time.Sleep(500 * time.Millisecond)
			}
			if be == "sftp" {
				// SFTP is fine here
			}
			var basePath string
			if be == "s3" {
				basePath = "vfs:/s3/testbucket"
			} else {
				basePath = fmt.Sprintf("vfs:/%s", be)
			}
			zipPath := path.Join(basePath, "test.zip")

			if writefs.IsWriteable(vfs, zipPath) {
				subFS, err := vfs.SubCreate(zipPath)
				if err != nil {
					t.Fatalf("[%s] failed to create zip via vfs.Sub: %v", be, err)
				}
				if _, err := writefs.WriteFile(subFS, "test.txt", []byte("hello zip test.zip")); err != nil {
					t.Fatalf("[%s] failed to write test.txt to zip: %v", be, err)
				}
				if _, err := writefs.WriteFile(subFS, "sub/subtest.txt", []byte("hello sub zip")); err != nil {
					t.Fatalf("[%s] failed to write sub/subtest.txt to zip: %v", be, err)
				}
				if _, err := writefs.WriteFile(subFS, "sub/deep/deeptest.txt", []byte("hello deep zip")); err != nil {
					t.Fatalf("[%s] failed to write sub/deep/deeptest.txt to zip: %v", be, err)
				}
				if closer, ok := subFS.(io.Closer); ok {
					closer.Close()
				}
				//time.Sleep(1 * time.Second) // Give S3 a moment

				// Check for checksum files
				for _, alg := range []checksum.DigestAlgorithm{checksum.DigestSHA512} {
					checksumFile := zipPath + "." + string(alg)
					if _, err := vfs.Stat(checksumFile); err != nil {
						t.Errorf("[%s] expected checksum file '%s' to exist, got error: %v", be, checksumFile, err)
					} else {
						if be == "s3" {
							t.Logf("[%s] Skipping checksum content verification for S3 due to GoFakeS3 chunked encoding issues", be)
							continue
						}
						data, err := vfs.ReadFile(checksumFile)
						if err != nil {
							t.Errorf("[%s] failed to read checksum file '%s': %v", be, checksumFile, err)
						} else {
							t.Logf("[%s] checksum file '%s' content: %s", be, checksumFile, string(data))
							dataStr := string(data)
							// Handle S3 Chunked Encoding metadata if present
							if strings.Contains(dataStr, ";chunk-signature=") {
								parts := strings.Split(dataStr, "\n")
								if len(parts) > 1 {
									dataStr = strings.Join(parts[1:], "\n")
								}
							}
							parts := strings.Fields(dataStr)
							if len(parts) < 1 {
								t.Errorf("[%s] invalid checksum file format: %s", be, string(data))
							} else {
								expectedChecksum := parts[0]
								zipData, err := vfs.ReadFile(zipPath)
								if err != nil {
									t.Errorf("[%s] failed to read zip file '%s' for checksum verification: %v", be, zipPath, err)
								} else {
									csWriter, err := checksum.NewChecksumWriter([]checksum.DigestAlgorithm{alg}, io.Discard)
									if err != nil {
										t.Errorf("[%s] failed to create checksum writer: %v", be, err)
									} else {
										if _, err := io.Copy(csWriter, bytes.NewReader(zipData)); err != nil {
											t.Errorf("[%s] failed to write zip data to checksum writer: %v", be, err)
										} else {
											if err := csWriter.Close(); err != nil {
												t.Errorf("[%s] failed to close checksum writer: %v", be, err)
											} else {
												actualChecksums, err := csWriter.GetChecksums()
												if err != nil {
													t.Errorf("[%s] failed to get checksums: %v", be, err)
												} else {
													actualChecksum := actualChecksums[alg]
													if actualChecksum != expectedChecksum {
														t.Errorf("[%s] checksum mismatch for '%s': expected '%s', got '%s'. Full checksum file content: '%s'", be, zipPath, expectedChecksum, actualChecksum, string(data))
													} else {
														t.Logf("[%s] checksum for '%s' is correct: %s", be, zipPath, actualChecksum)
													}
												}
											}
										}
									}
								}
							}
						}
					}
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
					if e.Name() == path.Join(basePath, "/test.zip") {
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
				if e.Name() == zipPath+"/sub" {
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
				if e.Name() == zipPath+"/sub/deep" {
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
	s3Endpoint, _, s3AccessKey, s3SecretKey, _, s3Closer := setupS3Server(t)
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
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 2, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
		"os": &vfsrw.VFS{
			Name:        "os",
			Type:        "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 2, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			OS:          &vfsrw.OS{BaseDir: osTempDir},
		},
		"sftp": &vfsrw.VFS{
			Name:        "sftp",
			Type:        "sftp",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 2, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
		"s3": &vfsrw.VFS{
			Name:        "s3",
			Type:        "s3",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 2, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
			Name:        "web",
			Type:        "web",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 2, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
			if be == "s3" {
				//t.Skip("Skipping S3 in CacheLimit test due to eventual consistency issues (file not visible immediately after write)")
				vfs.MkDir("vfs://s3/testbucket")
				//time.Sleep(500 * time.Millisecond)
			}
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
					if writefs.IsWriteable(vfs, zp) {
						subFS, err := vfs.SubCreate(zp)
						if err != nil {
							t.Fatalf("failed to create zip %s via vfs.Sub: %v", zp, err)
						}
						var content string
						switch i {
						case 1:
							content = "hello zip test1.zip"
						case 2:
							content = "hello zip test2.zip"
						case 3:
							content = "hello zip test3.zip"
						default:
							content = fmt.Sprintf("hello zip test%d.zip", i)
						}
						if _, err := writefs.WriteFile(subFS, "test.txt", []byte(content)); err != nil {
							t.Fatalf("failed to write test.txt to zip %s: %v", zp, err)
						}
						if closer, ok := subFS.(io.Closer); ok {
							closer.Close()
						}
					}
				}
				if be == "s3" {
					// time.Sleep(2 * time.Second) // Wait for S3 consistency after writing all files
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
				for i, zp := range []string{zipPath1, zipPath2, zipPath3} {
					subFS, err := vfs.SubCreate(zp)
					if err != nil {
						t.Fatalf("failed to create zip %s via vfs.Sub: %v", zp, err)
					}
					var content string
					switch i {
					case 0:
						content = "hello zip test1.zip"
					case 1:
						content = "hello zip test2.zip"
					case 2:
						content = "hello zip test3.zip"
					}
					if _, err := writefs.WriteFile(subFS, "test.txt", []byte(content)); err != nil {
						t.Fatalf("failed to write test.txt to zip %s: %v", zp, err)
					}
					if closer, ok := subFS.(io.Closer); ok {
						closer.Close()
					}
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
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 2, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	zipPath := "vfs://mem/test.zip"
	subFS, err := vfs.SubCreate(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip via vfs.Sub: %v", err)
	}
	if _, err := writefs.WriteFile(subFS, "test.txt", []byte("hello zip test.zip")); err != nil {
		t.Fatalf("failed to write test.txt to zip: %v", err)
	}
	if _, err := writefs.WriteFile(subFS, "sub/subtest.txt", []byte("hello sub zip")); err != nil {
		t.Fatalf("failed to write sub/subtest.txt to zip: %v", err)
	}
	if _, err := writefs.WriteFile(subFS, "sub/deep/deeptest.txt", []byte("hello deep zip")); err != nil {
		t.Fatalf("failed to write sub/deep/deeptest.txt to zip: %v", err)
	}
	if closer, ok := subFS.(io.Closer); ok {
		closer.Close()
	}

	// 1. Stat auf das Zip selbst
	fi, err := vfs.Stat(zipPath)
	if err != nil {
		t.Fatalf("Stat on test.zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected test.zip to be a directory")
	}

	// 2. Stat auf Datei im Zip
	fi, err = vfs.Stat(zipPath + "/test.txt")
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
	fi, err = vfs.Stat(zipPath + "/sub")
	if err != nil {
		t.Fatalf("Stat on sub in zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected sub to be a directory")
	}

	// 4. Stat auf tiefes Verzeichnis im Zip
	fi, err = vfs.Stat(zipPath + "/sub/deep")
	if err != nil {
		t.Fatalf("Stat on sub/deep in zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected sub/deep to be a directory")
	}

	// 5. Stat auf Datei in tiefem Verzeichnis
	fi, err = vfs.Stat(zipPath + "/sub/deep/deeptest.txt")
	if err != nil {
		t.Fatalf("Stat on deeptest.txt in zip failed: %v", err)
	}
	if fi.IsDir() {
		t.Errorf("expected deeptest.txt to be a file")
	}

	// 6. Stat auf nicht existierende Datei im Zip
	_, err = vfs.Stat(zipPath + "/nonexistent.txt")
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
	s3Endpoint, _, s3AccessKey, s3SecretKey, _, s3Closer := setupS3Server(t)
	defer s3Closer()

	// 4. Web Setup
	zipData := createCustomTestZip("test.txt", "hello world")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, r.URL.Path, time.Now(), bytes.NewReader(zipData))
	}))
	defer ts.Close()

	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 20, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
		"os": &vfsrw.VFS{
			Name:        "os",
			Type:        "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 20, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			OS:          &vfsrw.OS{BaseDir: osTempDir},
		},
		"sftp": &vfsrw.VFS{
			Name:        "sftp",
			Type:        "sftp",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 20, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
			SFTP: &vfsrw.SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 20,
			},
		},
		"s3": &vfsrw.VFS{
			Name:        "s3",
			Type:        "s3",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 20, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
			Name:        "web",
			Type:        "web",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 20, Digests: []checksum.DigestAlgorithm{checksum.DigestSHA512}},
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
		t.Run("Backend_"+be, func(t *testing.T) {
			basePath := fmt.Sprintf("vfs://%s/", be)
			if be == "s3" {
				basePath = fmt.Sprintf("vfs://%s/testbucket/", be)
			}
			if !writefs.IsWriteable(vfs, basePath) {
				t.Skip("Skip read-only backends in this test setup")
			}
			for i := range numZips {
				name := basePath + fmt.Sprintf("test%d.zip", i)
				subFS, err := vfs.SubCreate(name)
				if err != nil {
					t.Fatalf("[%s] failed to create zip %s via vfs.Sub: %v", be, name, err)
				}
				if _, err := writefs.WriteFile(subFS, "test.txt", []byte("hello world")); err != nil {
					t.Fatalf("[%s] failed to write test.txt to zip %s: %v", be, name, err)
				}
				if closer, ok := subFS.(io.Closer); ok {
					closer.Close()
				}
			}
		})
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
				if backend == "s3" || backend == "sftp" || backend == "web" {
					continue
				}
				zipID := (workerID + j) % numZips

				basePath := fmt.Sprintf("vfs://%s/", backend)
				if backend == "s3" {
					basePath = fmt.Sprintf("vfs://%s/testbucket/", backend)
				}
				path := basePath + fmt.Sprintf("test%d.zip/test.txt", zipID)

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
				dirPath := basePath + fmt.Sprintf("test%d.zip", zipID)
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

func TestZipAsFolder_UnclosedFiles(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	// OS setup for temporary directory
	osTempDir := setupOSTempDir(t)
	defer os.RemoveAll(osTempDir)

	// Create two different zip files
	zipData1 := createCustomTestZip("file1.txt", "content of file 1")
	zipData2 := createCustomTestZip("file2.txt", "content of file 2")

	os.WriteFile(filepath.Join(osTempDir, "test1.zip"), zipData1, 0644)
	os.WriteFile(filepath.Join(osTempDir, "test2.zip"), zipData2, 0644)

	// Configuration with CacheSize: 1
	cfg := vfsrw.Config{
		"os": &vfsrw.VFS{
			Name: "os",
			Type: "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{
				Enabled:   true,
				CacheSize: 1, // Only one zip file in cache at a time
			},
			OS: &vfsrw.OS{BaseDir: osTempDir},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	// 1. Open a file from the first zip file
	f1, err := vfs.Open("vfs://os/test1.zip/file1.txt")
	if err != nil {
		t.Fatalf("failed to open file1.txt in test1.zip: %v", err)
	}
	// We intentionally do NOT close f1 here yet.

	// 2. Access the second zip file.
	// Since CacheSize=1, test1.zip will be evicted from the otter cache.
	data2, err := vfs.ReadFile("vfs://os/test2.zip/file2.txt")
	if err != nil {
		t.Fatalf("failed to read file2.txt in test2.zip: %v", err)
	}
	if string(data2) != "content of file 2" {
		t.Errorf("expected 'content of file 2', got '%s'", string(data2))
	}

	// 3. Verify that f1 (from the evicted zip file) is still readable.
	// If the zip file had been closed immediately upon eviction, this would now fail.
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, f1); err != nil {
		t.Errorf("failed to read from f1 after test1.zip was evicted from cache: %v", err)
	}
	if buf.String() != "content of file 1" {
		t.Errorf("expected 'content of file 1', got '%s'", buf.String())
	}

	// 4. Now close f1
	if err := f1.Close(); err != nil {
		t.Errorf("failed to close f1: %v", err)
	}

	t.Log("Successfully verified that unclosed files keep the Zip-FS alive after cache eviction.")
}

func TestZipAsFolder_Timeout(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	osTempDir := setupOSTempDir(t)
	defer os.RemoveAll(osTempDir)

	zipData := createCustomTestZip("file.txt", "content")
	os.WriteFile(filepath.Join(osTempDir, "test.zip"), zipData, 0644)

	// Short timeout for the test
	timeout := 500 * time.Millisecond
	cfg := vfsrw.Config{
		"os": &vfsrw.VFS{
			Name: "os",
			Type: "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{
				Enabled:   true,
				CacheSize: 10,
				Timeout:   config.Duration(timeout),
			},
			OS: &vfsrw.OS{BaseDir: osTempDir},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	// 1. Access zip file to load it into the cache
	_, err = vfs.ReadFile("vfs://os/test.zip/file.txt")
	if err != nil {
		t.Fatalf("failed to read file.txt in test.zip: %v", err)
	}

	// 2. Wait longer than the timeout + some buffer for the ClearUnlocked loop
	time.Sleep(timeout * 3)

	// 3. Access again. If the timeout worked, the zip file was unloaded
	// Since we have no direct access to the internal state, we check the logs
	// or trust that the code path was executed.
	// Since we cannot directly check ClearUnlocked (internal), this test is more of a "sanity check".
	// To really prove it, we would need to use an interface or a mock logger.

	_, err = vfs.ReadFile("vfs://os/test.zip/file.txt")
	if err != nil {
		t.Fatalf("failed to read file.txt after timeout: %v", err)
	}
}

func TestZipAsFolder_Finalizer(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	osTempDir := setupOSTempDir(t)
	defer os.RemoveAll(osTempDir)

	zipData := createCustomTestZip("file.txt", "content")
	os.WriteFile(filepath.Join(osTempDir, "test.zip"), zipData, 0644)

	cfg := vfsrw.Config{
		"os": &vfsrw.VFS{
			Name: "os",
			Type: "os",
			ZipAsFolder: &vfsrw.ZipAsFolder{
				Enabled:   true,
				CacheSize: 10,
			},
			OS: &vfsrw.OS{BaseDir: osTempDir},
		},
	}

	// We create the VFS in an anonymous function so it can be released for GC afterwards
	createAndUseVFS := func() {
		vfs, err := vfsrw.NewFS(cfg, _logger)
		if err != nil {
			t.Errorf("failed to create vfs: %v", err)
			return
		}
		// Access to load ZipFS
		_, _ = vfs.ReadFile("vfs://os/test.zip/file.txt")
		// vfs becomes unreachable here at the end of the function
	}

	createAndUseVFS()

	// Trigger GC several times to execute finalizer/cleanup
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
	}

	// This test primarily verifies that the finalizer code doesn't panic or lead to deadlocks.
	// Real proof that the channel was closed is difficult without internal hooks.
}
