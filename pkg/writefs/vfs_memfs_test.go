package writefs_test

import (
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/je4/filesystem/v3/pkg/vfsrw"
	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestVFS_MemFS(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger

	cfg := vfsrw.Config{
		"testmem": &vfsrw.VFS{
			Name:  "testmem",
			Type:  "memfs",
			MemFS: &vfsrw.MemFS{},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	testData := []byte("vfs memfs test data")
	testFile := "vfs://testmem/test.txt"

	// Write via writefs.Create
	f, err := writefs.Create(vfs, testFile)
	if err != nil {
		t.Fatalf("failed to create file on vfs via writefs: %v", err)
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

	// Read via fs.ReadFile (vfs implements ReadFileFS)
	data, err := fs.ReadFile(vfs, testFile)
	if err != nil {
		t.Fatalf("failed to read from vfs: %v", err)
	}
	if string(data) != string(testData) {
		t.Fatalf("expected %s, got %s", string(testData), string(data))
	}

	// Stat via fs.Stat
	fi, err := fs.Stat(vfs, testFile)
	if err != nil {
		t.Fatalf("failed to stat vfs file: %v", err)
	}
	fmt.Printf("File Name: %s, Size: %d\n", fi.Name(), fi.Size())

	// Write via writefs.WriteFile
	testFile2 := "vfs://testmem/test2.txt"
	testData2 := []byte("more test data")
	_, err = writefs.WriteFile(vfs, testFile2, testData2)
	if err != nil {
		t.Fatalf("failed to WriteFile: %v", err)
	}

	// Read back testFile2
	data2, err := fs.ReadFile(vfs, testFile2)
	if err != nil {
		t.Fatalf("failed to read testFile2: %v", err)
	}
	if string(data2) != string(testData2) {
		t.Fatalf("data mismatch for testFile2: expected %s, got %s", string(testData2), string(data2))
	}

	// Copy via writefs.Copy
	copyFile := "vfs://testmem/copy.txt"
	_, err = writefs.Copy(vfs, testFile, vfs, copyFile)
	if err != nil {
		t.Fatalf("failed to copy file via writefs.Copy: %v", err)
	}

	// Read copied file
	data, err = fs.ReadFile(vfs, copyFile)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(data) != string(testData) {
		t.Fatalf("copied data mismatch: expected %s, got %s", string(testData), string(data))
	}

	// MkDir via writefs.MkDir
	testDir := "vfs://testmem/newdir"
	err = writefs.MkDir(vfs, testDir)
	if err != nil {
		t.Fatalf("failed to MkDir: %v", err)
	}

	// ReadDir via fs.ReadDir
	entries, err := fs.ReadDir(vfs, "vfs://testmem/")
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == "newdir" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("directory 'newdir' not found in listing")
	}

	// Remove via writefs.Remove
	err = writefs.Remove(vfs, testFile)
	if err != nil {
		t.Fatalf("failed to remove original file: %v", err)
	}

	// Stat original file should fail
	_, err = fs.Stat(vfs, testFile)
	if err == nil {
		t.Fatalf("original file still exists after removal")
	}
}
