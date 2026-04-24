package memFS

import (
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestMemFS_Basic(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger

	mfs, err := NewFS(_logger)
	if err != nil {
		t.Fatal(err)
	}

	testData := []byte("hello world")
	testFile := "test.txt"

	// Write
	n, err := mfs.WriteFile(testFile, testData)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if n != int64(len(testData)) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Read
	data, err := fs.ReadFile(mfs, testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != string(testData) {
		t.Fatalf("expected %s, got %s", string(testData), string(data))
	}

	// Stat
	fi, err := fs.Stat(mfs, testFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if fi.Name() != testFile {
		t.Fatalf("expected name %s, got %s", testFile, fi.Name())
	}

	// ReadDir
	entries, err := fs.ReadDir(mfs, ".")
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == testFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("file not found in directory listing")
	}

	// Rename
	newFile := "renamed.txt"
	err = mfs.Rename(testFile, newFile)
	if err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}

	// Check if old exists
	_, err = fs.Stat(mfs, testFile)
	if err == nil {
		t.Fatalf("old file still exists after rename")
	}

	// Check if new exists
	_, err = fs.Stat(mfs, newFile)
	if err != nil {
		t.Fatalf("new file does not exist after rename: %v", err)
	}

	// Remove
	err = writefs.Remove(mfs, newFile)
	if err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}

	// Check if exists
	_, err = fs.Stat(mfs, newFile)
	if err == nil {
		t.Fatalf("file still exists after removal")
	}
}

func TestMemFS_MkDir(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger

	mfs, err := NewFS(_logger)
	if err != nil {
		t.Fatal(err)
	}

	dirName := "subdir/nested"
	err = mfs.MkDir(dirName)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	fi, err := fs.Stat(mfs, "subdir")
	if err != nil || !fi.IsDir() {
		t.Fatalf("subdir not created correctly")
	}

	fi, err = fs.Stat(mfs, dirName)
	if err != nil || !fi.IsDir() {
		t.Fatalf("nested dir not created correctly")
	}
}

func TestMemFS_FileInterfaces(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger

	mfs, err := NewFS(_logger)
	if err != nil {
		t.Fatal(err)
	}

	testData := []byte("0123456789")
	testFile := "interfaces.txt"

	_, err = mfs.WriteFile(testFile, testData)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	f, err := mfs.Open(testFile)
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
}
