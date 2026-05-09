package vfsrw

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/je4/filesystem/v3/pkg/writefs"
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

func TestZipAsFolder_Afero(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
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

	if _, err := writefs.WriteFile(vfs, "vfs://mem/test.zip", zipData); err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	// Test access into zip
	data, err := vfs.ReadFile("vfs://mem/test.zip/test.txt")
	if err != nil {
		t.Fatalf("failed to read file in zip: %v", err)
	}
	if string(data) != "hello zip" {
		t.Fatalf("expected 'hello zip', got '%s'", string(data))
	}

	data, err = vfs.ReadFile("vfs://mem/test.zip/sub/deep/deeptest.txt")
	if err != nil {
		t.Fatalf("failed to read deep file in zip: %v", err)
	}
	if string(data) != "hello deep zip" {
		t.Fatalf("expected 'hello deep zip', got '%s'", string(data))
	}
}

func TestZipAsFolder_OS(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	tempDir, err := os.MkdirTemp("", "vfs_os_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tempDir = filepath.ToSlash(tempDir)
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

	zipData := createTestZip()

	if _, err := writefs.WriteFile(vfs, "vfs://os/test.zip", zipData); err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	// Test access into zip
	t.Logf("reading vfs://os/test.zip/test.txt")
	fi_zip, err_zip := vfs.Stat("vfs://os/test.zip")
	t.Logf("Stat('vfs://os/test.zip') -> %v, %v", fi_zip, err_zip)

	fi_direct, err_direct := os.Stat(filepath.Join(tempDir, "test.zip"))
	t.Logf("os.Stat('%s') -> %v, %v", filepath.Join(tempDir, "test.zip"), fi_direct, err_direct)

	data, err := vfs.ReadFile("vfs://os/test.zip/test.txt")
	if err != nil {
		t.Fatalf("failed to read file in zip: %v", err)
	}
	if string(data) != "hello zip" {
		t.Fatalf("expected 'hello zip', got '%s'", string(data))
	}

	t.Logf("reading vfs://os/test.zip/sub/deep/deeptest.txt")
	data, err = vfs.ReadFile("vfs://os/test.zip/sub/deep/deeptest.txt")
	if err != nil {
		t.Fatalf("failed to read deep file in zip: %v", err)
	}
	if string(data) != "hello deep zip" {
		t.Fatalf("expected 'hello deep zip', got '%s'", string(data))
	}

	// Test Stat on zip itself as folder
	t.Logf("stating vfs://os/test.zip")
	fi, err := vfs.Stat("vfs://os/test.zip")
	if err != nil {
		// Try with trailing slash
		t.Logf("stat on vfs://os/test.zip failed: %v, trying with trailing slash", err)
		fi, err = vfs.Stat("vfs://os/test.zip/")
	}
	if err != nil {
		t.Fatalf("stat on zip failed: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("expected zip to be treated as directory")
	}
}

func TestZipAsFolder_SFTP(t *testing.T) {
	t.Skip("SFTP test requires a running SFTP server")
}

func TestZipAsFolder_ReadDir(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
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
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
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
	if _, err := writefs.WriteFile(vfs, "vfs://mem/1.zip", zipData); err != nil {
		t.Fatalf("failed to write zip file 1: %v", err)
	}
	if _, err := writefs.WriteFile(vfs, "vfs://mem/2.zip", zipData); err != nil {
		t.Fatalf("failed to write zip file 2: %v", err)
	}
	if _, err := writefs.WriteFile(vfs, "vfs://mem/3.zip", zipData); err != nil {
		t.Fatalf("failed to write zip file 3: %v", err)
	}

	// Zugriff auf 1.zip -> Geladen (Cache: [1])
	t.Log("Accessing 1.zip")
	if _, err := vfs.ReadFile("vfs://mem/1.zip/test.txt"); err != nil {
		t.Fatalf("failed to read 1.zip: %v", err)
	}

	// Zugriff auf 2.zip -> Geladen (Cache: [1, 2])
	t.Log("Accessing 2.zip")
	if _, err := vfs.ReadFile("vfs://mem/2.zip/test.txt"); err != nil {
		t.Fatalf("failed to read 2.zip: %v", err)
	}

	// Zugriff auf 3.zip -> Geladen, 1.zip sollte verdrängt werden (Cache: [2, 3])
	t.Log("Accessing 3.zip (should evict 1.zip)")
	if _, err := vfs.ReadFile("vfs://mem/3.zip/test.txt"); err != nil {
		t.Fatalf("failed to read 3.zip: %v", err)
	}

	// Erneuter Zugriff auf 1.zip -> Sollte neu geladen werden (Cache: [3, 1], 2.zip verdrängt)
	t.Log("Accessing 1.zip again (should evict 2.zip)")
	if _, err := vfs.ReadFile("vfs://mem/1.zip/test.txt"); err != nil {
		t.Fatalf("failed to read 1.zip again: %v", err)
	}
}

func TestZipAsFolder_Stat(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
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
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := Config{
		"mem": &VFS{
			Name:             "mem",
			Type:             "afero",
			ZipAsFolderCache: 5,
			Afero:            &Afero{BaseDir: "mem://"},
		},
	}
	vfs, err := NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	numZips := 10
	zipData := createCustomTestZip("test.txt", "hello world")
	for i := range numZips {
		name := fmt.Sprintf("vfs://mem/test%d.zip", i)
		if _, err := writefs.WriteFile(vfs, name, zipData); err != nil {
			t.Fatalf("failed to write zip file %d: %v", i, err)
		}
	}

	numWorkers := 20
	numIterations := 50
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			for j := range numIterations {
				zipID := (workerID + j) % numZips
				path := fmt.Sprintf("vfs://mem/test%d.zip/test.txt", zipID)

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
				dirPath := fmt.Sprintf("vfs://mem/test%d.zip", zipID)
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
