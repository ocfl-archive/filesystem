package vfsrw_test

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/rs/zerolog"
)

func TestVFS_SubZip(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	// 1. Configure VFS with MemFS.
	// IMPORTANT: ZipAsFolder on the FS itself is NOT necessary here,
	// because we want to test vfsrw.Sub, which triggers the ZIP logic
	// if the config allows it.
	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true}, // Allows vfsrw.Sub to do ZIP handling
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	// 2. Define ZIP file path
	zipFile := "vfs://mem/test.zip"

	// 3. Call vfs.Sub. Since test.zip doesn't exist yet, vfsrw.Sub
	// should create a new zipfsrw (Write-Only/Create) if ZipAsFolder is active.
	subFS, err := vfs.SubCreate(zipFile)
	if err != nil {
		t.Fatalf("failed to call vfs.Sub('%s'): %v", zipFile, err)
	}
	if subFS == nil {
		t.Fatalf("vfs.Sub('%s') returned nil, nil", zipFile)
	}

	// 4. Write test data in the sub-filesystem
	testFileName := "hello.txt"
	testContent := []byte("hello from sub zip")

	// We use writefs.WriteFile to ensure we use write operations
	n, err := writefs.WriteFile(subFS, testFileName, testContent)
	if err != nil {
		t.Fatalf("failed to write file in sub zip: %v", err)
	}
	if n != int64(len(testContent)) {
		t.Fatalf("wrote %d bytes, expected %d", n, len(testContent))
	}

	// 5. Close ZIP (if it's a closer) to ensure everything is written to the underlying FS.
	if closer, ok := subFS.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("failed to close sub zip fs: %v", err)
		}
	} else {
		t.Log("subFS does not implement io.Closer")
	}

	// 6. Verify that the ZIP file exists in the main VFS
	if _, err := fs.Stat(vfs, zipFile); err != nil {
		t.Fatalf("zip file does not exist after write: %v", err)
	}

	// 7. Now open the ZIP file again (Read-Only) and check content
	// Since vfsrw.Sub now uses zipfsw (write-only), we can no longer read via Sub.
	// We read the ZIP file as a whole and use archive/zip for verification.
	t.Log("Verifying ZIP content manually since vfsrw.Sub is now write-only")

	zipData, err := fs.ReadFile(vfs, zipFile)
	if err != nil {
		t.Fatalf("failed to read zip file: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}

	found := false
	for _, f := range zr.File {
		if f.Name == testFileName {
			found = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open file in zip: %v", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("failed to read file in zip: %v", err)
			}
			if string(content) != string(testContent) {
				t.Fatalf("content mismatch: expected '%s', got '%s'", string(testContent), string(content))
			}
		}
	}
	if !found {
		t.Fatalf("file '%s' not found in zip", testFileName)
	}
}
