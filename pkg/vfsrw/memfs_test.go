package vfsrw

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestVFS_MemFS(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger

	cfg := Config{
		"testmem": &VFS{
			Name:  "testmem",
			Type:  "memfs",
			MemFS: &MemFS{},
		},
	}

	vfs, err := NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	testData := []byte("vfs memfs test data")
	// Da vFSRW Pfade normalerweise mit dem Namen des FS einleiten (testmem:// oder ähnlich, je nach Implementierung von getFS)
	// Ich schaue mir getFS in fs.go an, um das Pfadformat zu verstehen.
	// Meistens ist es "name:/path" oder "name/path".

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

func TestVFS_MemFS_Interface(t *testing.T) {
	var _ writefs.CreateFS = &vFSRW{}
	var _ fs.FS = &vFSRW{}
}
