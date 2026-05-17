package vfsrw_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/rs/zerolog"
)

func TestVFS_MemFS(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))

	cfg := vfsrw.Config{
		"testmem": &vfsrw.VFS{
			Name:  "testmem",
			Type:  "afero",
			Afero: &vfsrw.Afero{BaseDir: "mem://"},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	testData := []byte("vfs memfs test data")
	// Since vFSRW paths normally start with the name of the FS (testmem:// or similar, depending on implementation of getFS)
	// I'm looking at getFS in fs.go to understand the path format.
	// Usually it's "name:/path" or "name/path".

	testFile := "vfs://testmem/test.txt"

	// Write via Create
	f, err := vfs.Create(testFile)
	if err != nil {
		t.Fatalf("failed to create file on vfs: %v", err)
	}
	n, err := f.Write(testData)
	if err != nil {
		f.Close()
		t.Fatalf("failed to write to vfs file: %v", err)
	}
	if n != len(testData) {
		f.Close()
		t.Fatalf("wrote %d bytes, expected %d", n, len(testData))
	}
	f.Close()

	// Read via ReadFile
	data, err := vfs.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read from vfs: %v", err)
	}
	if string(data) != string(testData) {
		t.Fatalf("expected %s, got %s", string(testData), string(data))
	}

	// Stat
	fi, err := vfs.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to stat vfs file: %v", err)
	}
	fmt.Printf("File Name: %s, Size: %d\n", fi.Name(), fi.Size())

	// ReadDir
	entries, err := vfs.ReadDir("vfs://testmem/")
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == "test.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("file 'test.txt' not found in directory listing")
	}

	// IsWriteable
	if !vfs.IsWriteable(testFile) {
		t.Fatalf("expected file '%s' to be writeable", testFile)
	}

	// Copy
	copyFile := "vfs://testmem/copy.txt"
	_, err = writefs.Copy(vfs, testFile, vfs, copyFile)
	if err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}

	// Read copied file
	data, err = vfs.ReadFile(copyFile)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(data) != string(testData) {
		t.Fatalf("copied data mismatch: expected %s, got %s", string(testData), string(data))
	}

	// Remove (Delete)
	err = writefs.Remove(vfs, testFile)
	if err != nil {
		t.Fatalf("failed to remove original file: %v", err)
	}

	// Stat original file should fail
	_, err = vfs.Stat(testFile)
	if err == nil {
		t.Fatalf("original file still exists after removal")
	}

	// Remove copy
	err = vfs.Remove(copyFile)
	if err != nil {
		t.Fatalf("failed to remove copied file: %v", err)
	}
}

func TestVFS_MemFS_Zip(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))

	cfg := vfsrw.Config{
		"testmem": &vfsrw.VFS{
			Name:        "testmem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, CacheSize: 10},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	// Da zipasfolder schreibzugriff IN zips verweigert, aber das Erstellen der zip-Datei selbst erlaubt,
	// befüllen wir das zip manuell und schreiben es dann ins VFS.
	zipBuf := new(bytes.Buffer)
	zw := zip.NewWriter(zipBuf)
	fInZip, err := zw.Create("inner.txt")
	if err != nil {
		t.Fatalf("failed to create file in zip writer: %v", err)
	}
	_, err = fInZip.Write([]byte("inner data"))
	if err != nil {
		t.Fatalf("failed to write data to zip writer: %v", err)
	}
	zw.Close()

	// Jetzt das Zip ins VFS schreiben (auf das Basis-FS 'testmem', zipasfolder lässt dies nun zu, da zipPath leer ist)
	_, err = writefs.WriteFile(vfs, "vfs://testmem/test.zip", zipBuf.Bytes())
	if err != nil {
		t.Fatalf("failed to write zip file: %v", err)
	}

	testFile := "vfs://testmem/test.zip/inner.txt"
	fi, err := vfs.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat on file in zip failed: %v", err)
	}
	if fi.Name() != "inner.txt" {
		t.Fatalf("expected 'inner.txt', got '%s'", fi.Name())
	}

	f, err := vfs.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open file in zip: %v", err)
	}
	defer f.Close()

	fi2, err := f.Stat()
	if err != nil {
		t.Fatalf("f.Stat on file in zip failed: %v", err)
	}
	if fi2.Size() != int64(len("inner data")) {
		t.Fatalf("expected size %d, got %d", len("inner data"), fi2.Size())
	}

	fmt.Printf("VFS with Afero MemFS and Zip Support created and verified via Stat()\n")
}

func TestVFS_MemFS_FileInterfaces(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))

	cfg := vfsrw.Config{
		"testmem": &vfsrw.VFS{
			Name:  "testmem",
			Type:  "afero",
			Afero: &vfsrw.Afero{BaseDir: "mem://"},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	testData := []byte("0123456789")
	testFile := "vfs://testmem/interfaces.txt"

	_, err = writefs.WriteFile(vfs, testFile, testData)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	f, err := vfs.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	// Test Seeker
	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatal("file does not implement io.Seeker")
	}

	_, err = seeker.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("seek failed: %v", err)
	}

	buf := make([]byte, 2)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 2 || string(buf) != "56" {
		t.Fatalf("expected '56', got '%s'", string(buf))
	}

	// Test ReaderAt
	readerAt, ok := f.(io.ReaderAt)
	if !ok {
		t.Fatal("file does not implement io.ReaderAt")
	}

	bufAt := make([]byte, 3)
	n, err = readerAt.ReadAt(bufAt, 2)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 3 || string(bufAt) != "234" {
		t.Fatalf("expected '234', got '%s'", string(bufAt))
	}

	// Test Stat
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if fi.Name() != filepath.Base(testFile) {
		t.Fatalf("expected filename '%s', got '%s'", filepath.Base(testFile), fi.Name())
	}
	if fi.Size() != int64(len(testData)) {
		t.Fatalf("expected size %d, got %d", len(testData), fi.Size())
	}
}

func TestVFS_MemFS_Interface(t *testing.T) {
	var _ writefs.CreateFS = vfsrw.VFSRW(nil)
	var _ fs.FS = vfsrw.VFSRW(nil)
}
