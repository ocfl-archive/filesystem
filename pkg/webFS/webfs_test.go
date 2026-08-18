package webFS

import (
	"io"
	"os"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestWebFS_SeekerReaderAt(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	wfs, err := NewFS(
		"https://raw.githubusercontent.com/%%PATH%%",
		nil,
		false,
		_logger,
	)
	if err != nil {
		t.Fatal(err)
	}

	filename := "je4/utils/main/pkg/zLogger/zLogger.go"
	f, err := wfs.Open(filename)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatal("file does not implement io.Seeker")
	}

	readerAt, ok := f.(io.ReaderAt)
	if !ok {
		t.Fatal("file does not implement io.ReaderAt")
	}

	// Test ReadAt
	buf := make([]byte, 10)
	n, err := readerAt.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt(0) failed: %v", err)
	}
	if n != 10 {
		t.Fatalf("ReadAt(0) expected 10 bytes, got %d", n)
	}

	// Test Seek
	pos, err := seeker.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek(5, SeekStart) failed: %v", err)
	}
	if pos != 5 {
		t.Fatalf("Seek(5, SeekStart) expected pos 5, got %d", pos)
	}

	buf2 := make([]byte, 5)
	n, err = f.Read(buf2)
	if err != nil {
		t.Fatalf("Read after Seek failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("Read after Seek expected 5 bytes, got %d", n)
	}

	// Compare ReadAt(5) with Read() after Seek(5)
	buf3 := make([]byte, 5)
	_, err = readerAt.ReadAt(buf3, 5)
	if err != nil {
		t.Fatalf("ReadAt(5) failed: %v", err)
	}
	if string(buf2) != string(buf3) {
		t.Fatalf("Read after Seek and ReadAt mismatch: %q vs %q", string(buf2), string(buf3))
	}
}

func TestWebFS_BuildURL(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(os.Stderr))
	wfs, err := NewFS(
		"https://example.com/files/%%PATH%%",
		nil,
		false,
		_logger,
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard path without question mark",
			input:    "foo/bar/baz.txt",
			expected: "https://example.com/files/foo/bar/baz.txt",
		},
		{
			name:     "path with question mark / query string",
			input:    "foo/bar.pdf?download=true",
			expected: "https://example.com/files/foo/bar.pdf?download=true",
		},
		{
			name:     "path with spaces and query string",
			input:    "my folder/test file.txt?v=2",
			expected: "https://example.com/files/my%20folder/test%20file.txt?v=2",
		},
		{
			name:     "path with multiple query parameters",
			input:    "path/to/resource?param1=value1&param2=value2",
			expected: "https://example.com/files/path/to/resource?param1=value1&param2=value2",
		},
		{
			name:     "encoded %3F in filename is preserved as %3F",
			input:    "foo/file%3Fname.txt",
			expected: "https://example.com/files/foo/file%3Fname.txt",
		},
		{
			name:     "lowercase encoded %3f in filename is preserved as %3F",
			input:    "foo/file%3fname.txt",
			expected: "https://example.com/files/foo/file%3Fname.txt",
		},
		{
			name:     "encoded %3F in filename with query string",
			input:    "my%3Ffile.txt?param=1",
			expected: "https://example.com/files/my%3Ffile.txt?param=1",
		},
		{
			name:     "spaces, encoded %3F and query string combined",
			input:    "my folder/test%3Ffile.txt?v=2",
			expected: "https://example.com/files/my%20folder/test%3Ffile.txt?v=2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := wfs.Fullpath(tc.input)
			if err != nil {
				t.Fatalf("Fullpath(%q) returned unexpected error: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("Fullpath(%q) = %q, expected %q", tc.input, got, tc.expected)
			}
		})
	}
}
