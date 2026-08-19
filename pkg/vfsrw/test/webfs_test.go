package vfsrw_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
	"github.com/rs/zerolog"
)

var fixedModTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func setupMockHTTPServer(t *testing.T) (*httptest.Server, map[string][]byte) {
	files := map[string][]byte{
		"/test.txt":                             []byte("hello webfs world"),
		"/dir/nested.bin":                       []byte("nested binary content data 1234567890"),
		"/resource.pdf?version=1&download=true": []byte("pdf version 1 content"),
		"/file%3Fname.txt":                      []byte("content of file with encoded question mark"),
		"/item%26detail.txt":                    []byte("content of file with encoded ampersand"),
		"/folder/test%3Ffile.txt?v=2&mode=test": []byte("combined encoded char and query params content"),
		"/my%20folder/space%20file.txt?v=1&x=y": []byte("spaces and query parameters content"),
		"/search?tag=a%26b&category=books":      []byte("query param with encoded ampersand value"),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqURI := r.URL.RequestURI()
		content, ok := files[reqURI]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, r.URL.Path, fixedModTime, bytes.NewReader(content))
	}))

	t.Cleanup(func() {
		server.Close()
	})

	return server, files
}

func TestVFS_WebFS_Read(t *testing.T) {
	server, files := setupMockHTTPServer(t)

	var logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := vfsrw.Config{
		"webtest": &vfsrw.VFS{
			Name: "webtest",
			Type: "web",
			Web: &vfsrw.Web{
				BaseURI: server.URL + "/%%PATH%%",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	testPath := "vfs://webtest/test.txt"
	expectedContent := files["/test.txt"]

	// Test ReadFile
	data, err := vfs.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", testPath, err)
	}
	if !bytes.Equal(data, expectedContent) {
		t.Fatalf("ReadFile(%q) = %q, expected %q", testPath, string(data), string(expectedContent))
	}

	// Test Open
	f, err := vfs.Open(testPath)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", testPath, err)
	}
	defer f.Close()

	openData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("io.ReadAll from Open(%q) error: %v", testPath, err)
	}
	if !bytes.Equal(openData, expectedContent) {
		t.Fatalf("Open read data = %q, expected %q", string(openData), string(expectedContent))
	}

	// Test Stat on VFS
	fi, err := vfs.Stat(testPath)
	if err != nil {
		t.Fatalf("Stat(%q) error: %v", testPath, err)
	}
	if fi.Size() != int64(len(expectedContent)) {
		t.Fatalf("Stat(%q).Size() = %d, expected %d", testPath, fi.Size(), len(expectedContent))
	}
	if fi.Name() != "test.txt" {
		t.Fatalf("Stat(%q).Name() = %q, expected %q", testPath, fi.Name(), "test.txt")
	}
	if !fi.ModTime().Equal(fixedModTime) {
		t.Fatalf("Stat(%q).ModTime() = %v, expected %v", testPath, fi.ModTime(), fixedModTime)
	}

	// Test Stat from File
	ffi, err := f.Stat()
	if err != nil {
		t.Fatalf("f.Stat() error: %v", err)
	}
	if ffi.Size() != int64(len(expectedContent)) {
		t.Fatalf("f.Stat().Size() = %d, expected %d", ffi.Size(), len(expectedContent))
	}

	// Test nested file
	nestedPath := "vfs://webtest/dir/nested.bin"
	expectedNested := files["/dir/nested.bin"]
	nestedData, err := vfs.ReadFile(nestedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", nestedPath, err)
	}
	if !bytes.Equal(nestedData, expectedNested) {
		t.Fatalf("ReadFile(%q) = %q, expected %q", nestedPath, string(nestedData), string(expectedNested))
	}
}

func TestVFS_WebFS_QueryParamsAndEscaping(t *testing.T) {
	server, files := setupMockHTTPServer(t)

	var logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := vfsrw.Config{
		"webtest": &vfsrw.VFS{
			Name: "webtest",
			Type: "web",
			Web: &vfsrw.Web{
				BaseURI: server.URL + "/%%PATH%%",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	tests := []struct {
		name        string
		vfsPath     string
		expectedURI string
	}{
		{
			name:        "query string with multiple parameters",
			vfsPath:     "vfs://webtest/resource.pdf?version=1&download=true",
			expectedURI: "/resource.pdf?version=1&download=true",
		},
		{
			name:        "uppercase URL-encoded question mark in filename",
			vfsPath:     "vfs://webtest/file%3Fname.txt",
			expectedURI: "/file%3Fname.txt",
		},
		{
			name:        "lowercase URL-encoded question mark in filename normalized to uppercase %3F",
			vfsPath:     "vfs://webtest/file%3fname.txt",
			expectedURI: "/file%3Fname.txt",
		},
		{
			name:        "URL-encoded ampersand in filename",
			vfsPath:     "vfs://webtest/item%26detail.txt",
			expectedURI: "/item%26detail.txt",
		},
		{
			name:        "combined encoded question mark and query parameters",
			vfsPath:     "vfs://webtest/folder/test%3Ffile.txt?v=2&mode=test",
			expectedURI: "/folder/test%3Ffile.txt?v=2&mode=test",
		},
		{
			name:        "spaces and query parameters",
			vfsPath:     "vfs://webtest/my folder/space file.txt?v=1&x=y",
			expectedURI: "/my%20folder/space%20file.txt?v=1&x=y",
		},
		{
			name:        "encoded ampersand in query parameter value",
			vfsPath:     "vfs://webtest/search?tag=a%26b&category=books",
			expectedURI: "/search?tag=a%26b&category=books",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedContent, ok := files[tc.expectedURI]
			if !ok {
				t.Fatalf("setup error: expected URI %q not in mock files", tc.expectedURI)
			}

			// Test ReadFile
			data, err := vfs.ReadFile(tc.vfsPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error: %v", tc.vfsPath, err)
			}
			if !bytes.Equal(data, expectedContent) {
				t.Fatalf("ReadFile(%q) = %q, expected %q", tc.vfsPath, string(data), string(expectedContent))
			}

			// Test Stat
			fi, err := vfs.Stat(tc.vfsPath)
			if err != nil {
				t.Fatalf("Stat(%q) error: %v", tc.vfsPath, err)
			}
			if fi.Size() != int64(len(expectedContent)) {
				t.Fatalf("Stat(%q).Size() = %d, expected %d", tc.vfsPath, fi.Size(), len(expectedContent))
			}

			// Test Open & read
			f, err := vfs.Open(tc.vfsPath)
			if err != nil {
				t.Fatalf("Open(%q) error: %v", tc.vfsPath, err)
			}
			defer f.Close()

			openData, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("io.ReadAll from Open(%q) error: %v", tc.vfsPath, err)
			}
			if !bytes.Equal(openData, expectedContent) {
				t.Fatalf("Open(%q) read data = %q, expected %q", tc.vfsPath, string(openData), string(expectedContent))
			}
		})
	}
}

func TestVFS_WebFS_SeekerReaderAt(t *testing.T) {
	server, files := setupMockHTTPServer(t)

	var logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := vfsrw.Config{
		"webtest": &vfsrw.VFS{
			Name: "webtest",
			Type: "web",
			Web: &vfsrw.Web{
				BaseURI: server.URL + "/%%PATH%%",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	testPath := "vfs://webtest/dir/nested.bin"
	expectedContent := files["/dir/nested.bin"]

	f, err := vfs.Open(testPath)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", testPath, err)
	}
	defer f.Close()

	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatalf("file opened from %q does not implement io.Seeker", testPath)
	}

	readerAt, ok := f.(io.ReaderAt)
	if !ok {
		t.Fatalf("file opened from %q does not implement io.ReaderAt", testPath)
	}

	// Test ReadAt from offset 7
	bufAt := make([]byte, 10)
	n, err := readerAt.ReadAt(bufAt, 7)
	if err != nil {
		t.Fatalf("ReadAt(7) error: %v", err)
	}
	if n != 10 {
		t.Fatalf("ReadAt(7) expected 10 bytes, got %d", n)
	}
	expectedSlice := expectedContent[7:17]
	if !bytes.Equal(bufAt, expectedSlice) {
		t.Fatalf("ReadAt(7) = %q, expected %q", string(bufAt), string(expectedSlice))
	}

	// Test Seek(7, io.SeekStart)
	pos, err := seeker.Seek(7, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek(7, SeekStart) error: %v", err)
	}
	if pos != 7 {
		t.Fatalf("Seek(7, SeekStart) pos = %d, expected 7", pos)
	}

	bufRead := make([]byte, 10)
	n, err = io.ReadFull(f, bufRead)
	if err != nil {
		t.Fatalf("io.ReadFull after Seek error: %v", err)
	}
	if n != 10 {
		t.Fatalf("io.ReadFull after Seek expected 10 bytes, got %d", n)
	}
	if !bytes.Equal(bufRead, expectedSlice) {
		t.Fatalf("Read after Seek = %q, expected %q", string(bufRead), string(expectedSlice))
	}

	// Test Seek relative (io.SeekCurrent)
	pos, err = seeker.Seek(2, io.SeekCurrent)
	if err != nil {
		t.Fatalf("Seek(2, SeekCurrent) error: %v", err)
	}
	if pos != 19 {
		t.Fatalf("Seek(2, SeekCurrent) pos = %d, expected 19", pos)
	}

	bufCur := make([]byte, 5)
	n, err = io.ReadFull(f, bufCur)
	if err != nil {
		t.Fatalf("io.ReadFull after SeekCurrent error: %v", err)
	}
	if !bytes.Equal(bufCur, expectedContent[19:24]) {
		t.Fatalf("Read after SeekCurrent = %q, expected %q", string(bufCur), string(expectedContent[19:24]))
	}
}

func TestVFS_WebFS_Headers(t *testing.T) {
	receivedAuth := ""
	receivedCustom := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")

		if r.URL.Path == "/protected.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("protected secret content"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	var logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := vfsrw.Config{
		"webauth": &vfsrw.VFS{
			Name: "webauth",
			Type: "web",
			Web: &vfsrw.Web{
				BaseURI: server.URL + "/%%PATH%%",
				Header: map[string][]string{
					"Authorization":   {"Bearer test-token-12345"},
					"X-Custom-Header": {"my-header-value"},
				},
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	data, err := vfs.ReadFile("vfs://webauth/protected.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "protected secret content" {
		t.Fatalf("ReadFile content mismatch: got %q", string(data))
	}

	if receivedAuth != "Bearer test-token-12345" {
		t.Errorf("expected Authorization header 'Bearer test-token-12345', got %q", receivedAuth)
	}
	if receivedCustom != "my-header-value" {
		t.Errorf("expected X-Custom-Header 'my-header-value', got %q", receivedCustom)
	}
}

func TestVFS_WebFS_ReadOnly(t *testing.T) {
	server, _ := setupMockHTTPServer(t)

	var logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := vfsrw.Config{
		"webtest": &vfsrw.VFS{
			Name: "webtest",
			Type: "web",
			Web: &vfsrw.Web{
				BaseURI: server.URL + "/%%PATH%%",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	// IsWriteable should return false
	if vfs.IsWriteable("vfs://webtest/test.txt") {
		t.Errorf("IsWriteable('vfs://webtest/test.txt') = true, expected false")
	}

	// Create should return error
	_, err = vfs.Create("vfs://webtest/newfile.txt")
	if err == nil {
		t.Errorf("Create on read-only webfs expected error, got nil")
	}

	// MkDir should return error
	err = vfs.MkDir("vfs://webtest/newdir")
	if err == nil {
		t.Errorf("MkDir on read-only webfs expected error, got nil")
	}
}

func TestVFS_WebFS_NotFound(t *testing.T) {
	server, _ := setupMockHTTPServer(t)

	var logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	cfg := vfsrw.Config{
		"webtest": &vfsrw.VFS{
			Name: "webtest",
			Type: "web",
			Web: &vfsrw.Web{
				BaseURI: server.URL + "/%%PATH%%",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	nonExistent := "vfs://webtest/does_not_exist.txt"

	// ReadFile non-existent
	_, err = vfs.ReadFile(nonExistent)
	if err == nil {
		t.Errorf("ReadFile(%q) expected error for 404, got nil", nonExistent)
	}

	// Open non-existent
	_, err = vfs.Open(nonExistent)
	if err == nil {
		t.Errorf("Open(%q) expected error for 404, got nil", nonExistent)
	}

	// Stat non-existent
	_, err = vfs.Stat(nonExistent)
	if err == nil {
		t.Errorf("Stat(%q) expected error for 404, got nil", nonExistent)
	}
}
