package vfsrw_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/vfsrw"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestVFS_AferoFS(t *testing.T) {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	var _logger zLogger.ZLogger = new(zerolog.New(out))

	cfg := vfsrw.Config{
		"testafero": &vfsrw.VFS{
			Name: "testafero",
			Type: "afero",
			Afero: &vfsrw.Afero{
				BaseDir: "mem://",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
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

func TestVFS_AferoFS_ComplexSchemes(t *testing.T) {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	var _logger zLogger.ZLogger = new(zerolog.New(out))

	cfg := vfsrw.Config{
		"mem1": &vfsrw.VFS{
			Name: "mem1",
			Type: "afero",
			Afero: &vfsrw.Afero{
				BaseDir: "mem://",
			},
		},
		"ro": &vfsrw.VFS{
			Name: "ro",
			Type: "afero",
			Afero: &vfsrw.Afero{
				BaseDir: "ro://?base=vfs://mem1/",
			},
		},
		"cow": &vfsrw.VFS{
			Name: "cow",
			Type: "afero",
			Afero: &vfsrw.Afero{
				BaseDir: "cow://?base=vfs://ro/&layer=vfs://mem1/",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	assert.NoError(t, err)
	defer vfs.Close()

	t.Run("ReadOnly", func(t *testing.T) {
		testData := []byte("hello")
		_, err := writefs.WriteFile(vfs, "vfs://mem1/test.txt", testData)
		assert.NoError(t, err)

		// Read from RO
		data, err := vfs.ReadFile("vfs://ro/test.txt")
		assert.NoError(t, err)
		assert.Equal(t, testData, data)

		// Write to RO should fail
		_, err = writefs.WriteFile(vfs, "vfs://ro/fail.txt", []byte("fail"))
		assert.Error(t, err)
	})

	t.Run("CopyOnWrite", func(t *testing.T) {
		cowData := []byte("cow")
		_, err := writefs.WriteFile(vfs, "vfs://cow/cow.txt", cowData)
		assert.NoError(t, err)

		// Should be in cow (mem1 layer)
		data, err := vfs.ReadFile("vfs://mem1/cow.txt")
		assert.NoError(t, err)
		assert.Equal(t, cowData, data)
	})
}

func TestVFS_AferoFS_FileInterfaces(t *testing.T) {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	var _logger zLogger.ZLogger = new(zerolog.New(out))

	cfg := vfsrw.Config{
		"testafero": &vfsrw.VFS{
			Name: "testafero",
			Type: "afero",
			Afero: &vfsrw.Afero{
				BaseDir: "mem://",
			},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
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
