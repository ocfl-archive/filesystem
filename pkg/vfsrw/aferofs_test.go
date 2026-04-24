package vfsrw

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/je4/filesystem/v3/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestVFS_AferoFS(t *testing.T) {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	logger := zerolog.New(out)
	var _logger zLogger.ZLogger = &logger

	cfg := Config{
		"testafero": &VFS{
			Name: "testafero",
			Type: "afero",
			Afero: &Afero{
				BaseDir: "mem://",
			},
		},
	}

	vfs, err := NewFS(cfg, _logger)
	assert.NoError(t, err)
	defer vfs.Close()

	testData := []byte("vfs afero test data")
	testFile := "vfs://testafero/test.txt"

	// Write via Create
	f, err := vfs.Create(testFile)
	if assert.NoError(t, err) {
		n, err := f.Write(testData)
		assert.NoError(t, err)
		assert.Equal(t, len(testData), n)
		f.Close()
	}

	// Read via ReadFile
	data, err := vfs.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, testData, data)

	// Stat
	fi, err := vfs.Stat(testFile)
	assert.NoError(t, err)
	if fi != nil {
		fmt.Printf("File Name: %s, Size: %d\n", fi.Name(), fi.Size())
		assert.Equal(t, "test.txt", fi.Name())
	}

	// ReadDir
	entries, err := vfs.ReadDir("vfs://testafero/")
	assert.NoError(t, err)
	found := false
	for _, entry := range entries {
		if entry.Name() == "test.txt" {
			found = true
			break
		}
	}
	assert.True(t, found, "file 'test.txt' not found in directory listing")

	// Copy
	copyFile := "vfs://testafero/copy.txt"
	_, err = writefs.Copy(vfs, testFile, vfs, copyFile)
	assert.NoError(t, err)

	// Read copied file
	data, err = vfs.ReadFile(copyFile)
	assert.NoError(t, err)
	assert.Equal(t, testData, data)

	// Remove (Delete)
	err = writefs.Remove(vfs, testFile)
	assert.NoError(t, err)

	// Stat original file should fail
	_, err = vfs.Stat(testFile)
	assert.Error(t, err)

	// Remove copy
	err = vfs.Remove(copyFile)
	assert.NoError(t, err)
}

func TestVFS_AferoFS_FileInterfaces(t *testing.T) {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	logger := zerolog.New(out)
	var _logger zLogger.ZLogger = &logger

	cfg := Config{
		"testafero": &VFS{
			Name: "testafero",
			Type: "afero",
			Afero: &Afero{
				BaseDir: "mem://",
			},
		},
	}

	vfs, err := NewFS(cfg, _logger)
	assert.NoError(t, err)
	defer vfs.Close()

	testData := []byte("0123456789")
	testFile := "vfs://testafero/interfaces.txt"

	_, err = writefs.WriteFile(vfs, testFile, testData)
	assert.NoError(t, err)

	f, err := vfs.Open(testFile)
	assert.NoError(t, err)
	defer f.Close()

	// Test Seeker
	seeker, ok := f.(io.Seeker)
	assert.True(t, ok, "file does not implement io.Seeker")
	if ok {
		_, err = seeker.Seek(5, io.SeekStart)
		assert.NoError(t, err)

		buf := make([]byte, 2)
		n, err := f.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, "56", string(buf))
	}

	// Test ReaderAt
	readerAt, ok := f.(io.ReaderAt)
	assert.True(t, ok, "file does not implement io.ReaderAt")
	if ok {
		bufAt := make([]byte, 3)
		n, err := readerAt.ReadAt(bufAt, 2)
		assert.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, "234", string(bufAt))
	}
}
