package vfsrw

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sftp_test_server "github.com/JuniorGuerra/sftp_test_server"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/config"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func createTestZip() []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("test.txt")
	f.Write([]byte("hello zip"))
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

func runZipAsFolderTest(t *testing.T, vfs VFSRW, fsName string) {
	zipData := createTestZip()
	zipPath := fmt.Sprintf("vfs://%s/test.zip", fsName)

	if _, err := writefs.WriteFile(vfs, zipPath, zipData); err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	// Test access into zip
	testFile := zipPath + "/test.txt"
	data, err := vfs.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file in zip: %v", err)
	}
	if string(data) != "hello zip" {
		t.Fatalf("expected 'hello zip', got '%s'", string(data))
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

func TestZipAsFolder_Afero(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	cfg := Config{
		"mem": &VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolderCache: 10,
			Afero:            &Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := NewFS(cfg, _logger)
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
	cfg := Config{
		"os": &VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolderCache: 10,
			OS:               &OS{BaseDir: tempDir},
		},
	}
	vfs, err := NewFS(cfg, _logger)
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

	cfg := Config{
		"sftp": &VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolderCache: 10,
			SFTP: &SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
	}
	vfs, err := NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	runZipAsFolderTest(t, vfs, "sftp")
}

func TestZipAsFolder_ReadDir(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	cfg := Config{
		"mem": &VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolderCache: 10,
			Afero:            &Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	zipData := createTestZip()
	writefs.WriteFile(vfs, "vfs://mem/test.zip", zipData)

	entries, err := vfs.ReadDir("vfs://mem/")
	if err != nil {
		t.Fatalf("ReadDir on vfs://mem/ failed: %v", err)
	}
	foundZip := false
	for _, e := range entries {
		t.Logf("Entry in root: %s (IsDir: %v)", e.Name(), e.IsDir())
		if e.Name() == "test.zip" {
			foundZip = true
			if !e.IsDir() {
				t.Errorf("expected test.zip to be a directory in root listing")
			}
		}
	}
	if !foundZip {
		t.Errorf("test.zip not found in root directory listing")
	}

	// Nun IN das Zip schauen
	entries, err = vfs.ReadDir("vfs://mem/test.zip")
	if err != nil {
		t.Fatalf("ReadDir on zip failed: %v", err)
	}
	foundSub := false
	for _, e := range entries {
		if e.Name() == "sub" {
			foundSub = true
			if !e.IsDir() {
				t.Errorf("expected sub to be a directory")
			}
		}
	}
	if !foundSub {
		t.Errorf("sub directory not found in zip root")
	}

	// Noch tiefer schauen
	entries, err = vfs.ReadDir("vfs://mem/test.zip/sub")
	if err != nil {
		t.Fatalf("ReadDir on zip/sub failed: %v", err)
	}
	foundDeep := false
	for _, e := range entries {
		if e.Name() == "deep" {
			foundDeep = true
			if !e.IsDir() {
				t.Errorf("expected deep to be a directory")
			}
		}
	}
	if !foundDeep {
		t.Errorf("deep directory not found in zip/sub")
	}

	// Wir akzeptieren hier eine leere Liste, wenn keine expliziten Verzeichnisse da sind,
	// aber Stat muss funktionieren.
	fi, err := vfs.Stat("vfs://mem/test.zip/sub/deep/deeptest.txt")
	if err != nil {
		t.Errorf("Stat on deeptest.txt in zip failed: %v", err)
	} else {
		t.Logf("Stat on deeptest.txt in zip succeeded: %s, %d", fi.Name(), fi.Size())
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

	cfg := Config{
		"mem": &VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolderCache: 2,
			Afero:            &Afero{BaseDir: "mem://"},
		},
		"os": &VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolderCache: 2,
			OS:               &OS{BaseDir: osTempDir},
		},
		"sftp": &VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolderCache: 2,
			SFTP: &SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 3,
			},
		},
	}
	vfs, err := NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	zipData := createTestZip()

	backends := []string{"mem", "os", "sftp"}

	for _, be := range backends {
		t.Run("Backend_"+be, func(t *testing.T) {
			if _, err := writefs.WriteFile(vfs, fmt.Sprintf("vfs://%s/1.zip", be), zipData); err != nil {
				t.Fatalf("failed to write zip file 1: %v", err)
			}
			if _, err := writefs.WriteFile(vfs, fmt.Sprintf("vfs://%s/2.zip", be), zipData); err != nil {
				t.Fatalf("failed to write zip file 2: %v", err)
			}
			if _, err := writefs.WriteFile(vfs, fmt.Sprintf("vfs://%s/3.zip", be), zipData); err != nil {
				t.Fatalf("failed to write zip file 3: %v", err)
			}

			// Zugriff auf 1.zip -> Geladen (Cache: [1])
			t.Logf("[%s] Accessing 1.zip", be)
			if _, err := vfs.ReadFile(fmt.Sprintf("vfs://%s/1.zip/test.txt", be)); err != nil {
				t.Fatalf("failed to read 1.zip: %v", err)
			}

			// Zugriff auf 2.zip -> Geladen (Cache: [1, 2])
			t.Logf("[%s] Accessing 2.zip", be)
			if _, err := vfs.ReadFile(fmt.Sprintf("vfs://%s/2.zip/test.txt", be)); err != nil {
				t.Fatalf("failed to read 2.zip: %v", err)
			}

			// Zugriff auf 3.zip -> Geladen, 1.zip sollte verdrängt werden (Cache: [2, 3])
			t.Logf("[%s] Accessing 3.zip (should evict 1.zip)", be)
			if _, err := vfs.ReadFile(fmt.Sprintf("vfs://%s/3.zip/test.txt", be)); err != nil {
				t.Fatalf("failed to read 3.zip: %v", err)
			}

			// Erneuter Zugriff auf 1.zip -> Sollte neu geladen werden (Cache: [3, 1], 2.zip verdrängt)
			t.Logf("[%s] Accessing 1.zip again (should evict 2.zip)", be)
			if _, err := vfs.ReadFile(fmt.Sprintf("vfs://%s/1.zip/test.txt", be)); err != nil {
				t.Fatalf("failed to read 1.zip again: %v", err)
			}
		})
	}
}

func TestZipAsFolder_Stat(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))
	cfg := Config{
		"mem": &VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolderCache: 2,
			Afero:            &Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := NewFS(cfg, _logger)
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

	cfg := Config{
		"mem": &VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolderCache: 20,
			Afero:            &Afero{BaseDir: "mem://"},
		},
		"os": &VFS{
			Name:             "os",
			Type:             "os",
			ZipAsFolderCache: 20,
			OS:               &OS{BaseDir: osTempDir},
		},
		"sftp": &VFS{
			Name:             "sftp",
			Type:             "sftp",
			ZipAsFolderCache: 20,
			SFTP: &SFTP{
				Address:  config.EnvString(fmt.Sprintf("localhost:%d", port)),
				User:     config.EnvString(user),
				Password: config.EnvString(password),
				BaseDir:  "/",
				Sessions: 5,
			},
		},
	}
	vfs, err := NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	backends := []string{"mem", "os", "sftp"}
	numZips := 5
	zipData := createCustomTestZip("test.txt", "hello world")

	for _, be := range backends {
		for i := range numZips {
			name := fmt.Sprintf("vfs://%s/test%d.zip", be, i)
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
				path := fmt.Sprintf("vfs://%s/test%d.zip/test.txt", backend, zipID)

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
				dirPath := fmt.Sprintf("vfs://%s/test%d.zip", backend, zipID)
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
