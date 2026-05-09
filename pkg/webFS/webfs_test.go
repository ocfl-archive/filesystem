package webFS

import (
	"io"
	"os"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestWebFS_SeekerReaderAt(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger
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
