package mountFS

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestMountFS(t *testing.T) {
	// Create a base filesystem
	baseFS := fstest.MapFS{
		"file1.txt":     &fstest.MapFile{Data: []byte("content of file1")},
		"dir/file2.txt": &fstest.MapFile{Data: []byte("content of file2")},
	}

	// Create a mounted filesystem
	mountedFS := fstest.MapFS{
		"mountedFile.txt": &fstest.MapFile{Data: []byte("content of mountedFile")},
	}

	// Initialize MountFS
	mountFS := NewMountFS(baseFS)

	// Mount the mounted filesystem
	err := mountFS.Mount("/mount", mountedFS)
	if err != nil {
		t.Fatalf("failed to mount filesystem: %v", err)
	}

	// Test reading a file from the base filesystem
	data, err := fs.ReadFile(mountFS, "file1.txt")
	if err != nil {
		t.Fatalf("failed to read file from base filesystem: %v", err)
	}
	if string(data) != "content of file1" {
		t.Errorf("unexpected content: %s", string(data))
	}

	// Test reading a file from the mounted filesystem
	data, err = fs.ReadFile(mountFS, "/mount/mountedFile.txt")
	if err != nil {
		t.Fatalf("failed to read file from mounted filesystem: %v", err)
	}
	if string(data) != "content of mountedFile" {
		t.Errorf("unexpected content: %s", string(data))
	}

	// Test reading a directory
	entries, err := fs.ReadDir(mountFS, "/mount")
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "mountedFile.txt" {
		t.Errorf("unexpected directory entries: %v", entries)
	}
}
