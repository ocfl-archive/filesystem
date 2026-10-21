package zipfs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() zLogger.ZLogger {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	logger := zerolog.New(out).With().Timestamp().Logger()
	return &logger
}

// createTestZip creates an in-memory zip archive with various files and directory structures.
func createTestZip(t *testing.T) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// 1. Root file (deflate)
	w, err := zw.Create("root.txt")
	require.NoError(t, err)
	_, err = w.Write([]byte("root file content"))
	require.NoError(t, err)

	// 2. Uncompressed root file (store)
	hdrStore := &zip.FileHeader{
		Name:   "stored.txt",
		Method: zip.Store,
	}
	wStore, err := zw.CreateHeader(hdrStore)
	require.NoError(t, err)
	_, err = wStore.Write([]byte("0123456789abcdefghijklmnopqrstuvwxyz"))
	require.NoError(t, err)

	// 3. Explicit directory
	_, err = zw.Create("docs/")
	require.NoError(t, err)

	// 4. Files inside docs/
	w, err = zw.Create("docs/readme.md")
	require.NoError(t, err)
	_, err = w.Write([]byte("# Readme Content"))
	require.NoError(t, err)

	w, err = zw.Create("docs/license.txt")
	require.NoError(t, err)
	_, err = w.Write([]byte("MIT License"))
	require.NoError(t, err)

	// 5. Deeply nested file without explicit intermediate dir
	w, err = zw.Create("a/b/c/nested.txt")
	require.NoError(t, err)
	_, err = w.Write([]byte("nested content"))
	require.NoError(t, err)

	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestZipFS_BasicOperations(t *testing.T) {
	zipBytes := createTestZip(t)
	r := bytes.NewReader(zipBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(zipBytes)), "test.zip", logger)
	require.NoError(t, err)
	require.NotNil(t, zfs)

	t.Run("Stat root", func(t *testing.T) {
		fi, err := zfs.Stat(".")
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
		assert.Equal(t, ".", fi.Name())

		fiEmpty, err := zfs.Stat("")
		require.NoError(t, err)
		assert.True(t, fiEmpty.IsDir())
	})

	t.Run("Stat file", func(t *testing.T) {
		fi, err := zfs.Stat("root.txt")
		require.NoError(t, err)
		assert.False(t, fi.IsDir())
		assert.Equal(t, "root.txt", fi.Name())
		assert.Equal(t, int64(len("root file content")), fi.Size())
	})

	t.Run("Stat dir", func(t *testing.T) {
		fi, err := zfs.Stat("docs")
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
		assert.Equal(t, "docs", fi.Name())

		// Implicit intermediate directory
		fiNested, err := zfs.Stat("a/b")
		require.NoError(t, err)
		assert.True(t, fiNested.IsDir())
		assert.Equal(t, "b", fiNested.Name())
	})

	t.Run("Stat non-existent", func(t *testing.T) {
		_, err := zfs.Stat("nonexistent.txt")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("ReadFile", func(t *testing.T) {
		data, err := zfs.ReadFile("root.txt")
		require.NoError(t, err)
		assert.Equal(t, "root file content", string(data))

		dataNested, err := zfs.ReadFile("a/b/c/nested.txt")
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(dataNested))

		_, err = zfs.ReadFile("nonexistent.txt")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("OpenRaw", func(t *testing.T) {
		f, hdr, err := zfs.OpenRaw("root.txt")
		require.NoError(t, err)
		require.NotNil(t, hdr)
		defer f.Close()

		rawBytes, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.NotEmpty(t, rawBytes)
	})
}

func TestZipFS_ReadDir_Pagination(t *testing.T) {
	zipBytes := createTestZip(t)
	r := bytes.NewReader(zipBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(zipBytes)), "test.zip", logger)
	require.NoError(t, err)

	t.Run("ReadDir root via FS", func(t *testing.T) {
		entries, err := zfs.ReadDir(".")
		require.NoError(t, err)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		assert.Equal(t, []string{"a", "docs", "root.txt", "stored.txt"}, names)
	})

	t.Run("ReadDir non-existent dir", func(t *testing.T) {
		_, err := zfs.ReadDir("invalid_dir")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("dirFile ReadDir pagination", func(t *testing.T) {
		f, err := zfs.Open("docs")
		require.NoError(t, err)
		defer f.Close()

		dirF, ok := f.(fs.ReadDirFile)
		require.True(t, ok)

		// Read in chunks of 1
		chunk1, err := dirF.ReadDir(1)
		require.NoError(t, err)
		require.Len(t, chunk1, 1)
		assert.Equal(t, "license.txt", chunk1[0].Name())

		chunk2, err := dirF.ReadDir(1)
		require.NoError(t, err)
		require.Len(t, chunk2, 1)
		assert.Equal(t, "readme.md", chunk2[0].Name())

		// Reading beyond available entries must return io.EOF
		chunk3, err := dirF.ReadDir(1)
		assert.ErrorIs(t, err, io.EOF)
		assert.Empty(t, chunk3)
	})

	t.Run("dirFile ReadDir non-positive n", func(t *testing.T) {
		f, err := zfs.Open(".")
		require.NoError(t, err)
		defer f.Close()

		dirF, ok := f.(fs.ReadDirFile)
		require.True(t, ok)

		allEntries, err := dirF.ReadDir(-1)
		require.NoError(t, err)
		assert.Len(t, allEntries, 4)

		// Subsequent ReadDir on exhausted handle returns empty slice and nil error
		emptyEntries, err := dirF.ReadDir(0)
		require.NoError(t, err)
		assert.Empty(t, emptyEntries)
	})
}

func TestZipFS_Seek(t *testing.T) {
	zipBytes := createTestZip(t)
	r := bytes.NewReader(zipBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(zipBytes)), "test.zip", logger)
	require.NoError(t, err)

	t.Run("Seek stored uncompressed file", func(t *testing.T) {
		f, err := zfs.Open("stored.txt")
		require.NoError(t, err)
		defer f.Close()

		seeker, ok := f.(io.ReadSeeker)
		require.True(t, ok)

		// Seek to offset 10
		pos, err := seeker.Seek(10, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(10), pos)

		buf := make([]byte, 5)
		n, err := f.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "abcde", string(buf))
	})

	t.Run("Seek compressed file returns error", func(t *testing.T) {
		f, err := zfs.Open("root.txt")
		require.NoError(t, err)
		defer f.Close()

		seeker, ok := f.(io.Seeker)
		require.True(t, ok)

		_, err = seeker.Seek(5, io.SeekStart)
		assert.Error(t, err)
	})
}

func TestZipFS_RefCountAndDoubleClose(t *testing.T) {
	zipBytes := createTestZip(t)
	r := bytes.NewReader(zipBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(zipBytes)), "test.zip", logger)
	require.NoError(t, err)

	assert.Equal(t, int32(0), zfs.RefCount())

	// Open file increments refcount
	f1, err := zfs.Open("root.txt")
	require.NoError(t, err)
	assert.Equal(t, int32(1), zfs.RefCount())

	// Open directory increments refcount
	d1, err := zfs.Open("docs")
	require.NoError(t, err)
	assert.Equal(t, int32(2), zfs.RefCount())

	// Double close on file
	require.NoError(t, f1.Close())
	assert.Equal(t, int32(1), zfs.RefCount())
	require.NoError(t, f1.Close()) // Duplicate close must not decrement refcount again
	assert.Equal(t, int32(1), zfs.RefCount())

	// Double close on dirFile
	require.NoError(t, d1.Close())
	assert.Equal(t, int32(0), zfs.RefCount())
	require.NoError(t, d1.Close()) // Duplicate close must not decrement refcount again
	assert.Equal(t, int32(0), zfs.RefCount())
}

func TestZipFS_Sub(t *testing.T) {
	zipBytes := createTestZip(t)
	r := bytes.NewReader(zipBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(zipBytes)), "test.zip", logger)
	require.NoError(t, err)

	subFS, err := zfs.Sub("docs")
	require.NoError(t, err)

	data, err := fs.ReadFile(subFS, "readme.md")
	require.NoError(t, err)
	assert.Equal(t, "# Readme Content", string(data))

	entries, err := fs.ReadDir(subFS, ".")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestZipFS_Concurrency(t *testing.T) {
	zipBytes := createTestZip(t)
	r := bytes.NewReader(zipBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(zipBytes)), "test.zip", logger)
	require.NoError(t, err)

	const numGoroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 1. Read root file
				data, err := zfs.ReadFile("root.txt")
				if err != nil || string(data) != "root file content" {
					t.Errorf("worker %d: ReadFile root.txt failed: %v", workerID, err)
					return
				}

				// 2. Read nested file
				dataNested, err := zfs.ReadFile("a/b/c/nested.txt")
				if err != nil || string(dataNested) != "nested content" {
					t.Errorf("worker %d: ReadFile nested.txt failed: %v", workerID, err)
					return
				}

				// 3. Stat operations
				_, err = zfs.Stat("docs")
				if err != nil {
					t.Errorf("worker %d: Stat docs failed: %v", workerID, err)
					return
				}

				// 4. ReadDir
				entries, err := zfs.ReadDir("docs")
				if err != nil || len(entries) != 2 {
					t.Errorf("worker %d: ReadDir docs failed: %v (len %d)", workerID, err, len(entries))
					return
				}

				// 5. Open & Read stream concurrently
				f, err := zfs.Open("stored.txt")
				if err != nil {
					t.Errorf("worker %d: Open stored.txt failed: %v", workerID, err)
					return
				}
				buf := make([]byte, 10)
				_, err = f.Read(buf)
				if err != nil {
					f.Close()
					t.Errorf("worker %d: Read stored.txt failed: %v", workerID, err)
					return
				}
				if err := f.Close(); err != nil {
					t.Errorf("worker %d: Close stored.txt failed: %v", workerID, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int32(0), zfs.RefCount(), "all handles should be closed cleanly")
}

func TestZipFS_LargeArchivePerformance(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	const numFiles = 5000
	for i := 0; i < numFiles; i++ {
		w, err := zw.Create(fmt.Sprintf("folder%d/file%d.txt", i%50, i))
		require.NoError(t, err)
		_, err = w.Write([]byte(fmt.Sprintf("content of file %d", i)))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	archiveBytes := buf.Bytes()
	r := bytes.NewReader(archiveBytes)
	logger := newTestLogger()

	zfs, err := NewFS(r, int64(len(archiveBytes)), "large.zip", logger)
	require.NoError(t, err)

	// Direct O(1) lookup
	data, err := zfs.ReadFile("folder25/file25.txt")
	require.NoError(t, err)
	assert.Equal(t, "content of file 25", string(data))

	// Directory listing
	entries, err := zfs.ReadDir("folder25")
	require.NoError(t, err)
	assert.Len(t, entries, 100)
}
