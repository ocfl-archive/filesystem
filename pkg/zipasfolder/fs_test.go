package zipasfolder_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/zipasfolder"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLogger() zLogger.ZLogger {
	out := zerolog.ConsoleWriter{Out: io.Discard, TimeFormat: time.RFC3339}
	logger := zerolog.New(out).With().Timestamp().Logger()
	return &logger
}

func createZipArchive(files map[string]string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// countingFS wraps a MapFS and counts the number of Open calls for each file.
type countingFS struct {
	fstest.MapFS
	openCounts sync.Map // map[string]*atomic.Int64
}

func newCountingFS(m fstest.MapFS) *countingFS {
	return &countingFS{
		MapFS: m,
	}
}

func (c *countingFS) Open(name string) (fs.File, error) {
	val, _ := c.openCounts.LoadOrStore(name, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
	return c.MapFS.Open(name)
}

func (c *countingFS) getCount(name string) int64 {
	val, ok := c.openCounts.Load(name)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}

func TestConcurrentReads_SingleflightDeduplication(t *testing.T) {
	zipData := createZipArchive(map[string]string{
		"file1.txt":     "hello file1",
		"sub/file2.txt": "hello file2",
	})

	base := newCountingFS(fstest.MapFS{
		"archive.zip": &fstest.MapFile{
			Data: zipData,
			Mode: 0644,
		},
	})

	logger := createTestLogger()
	fsys, err := zipasfolder.NewFS(base, 10, time.Minute, true, logger)
	require.NoError(t, err)
	defer fsys.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	startBarrier := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			<-startBarrier

			data, err := fsys.ReadFile("archive.zip/file1.txt")
			assert.NoError(t, err)
			assert.Equal(t, "hello file1", string(data))

			data2, err := fsys.ReadFile("archive.zip/sub/file2.txt")
			assert.NoError(t, err)
			assert.Equal(t, "hello file2", string(data2))
		}()
	}

	close(startBarrier)
	wg.Wait()

	// Due to singleflight and cache, baseFS.Open("archive.zip") should be loaded very few times (ideally 1)
	opens := base.getCount("archive.zip")
	t.Logf("Total baseFS.Open calls for archive.zip under 50 concurrent workers: %d", opens)
	assert.LessOrEqual(t, opens, int64(3), "Expected singleflight deduplication to drastically limit open count")
}

func TestConcurrentReads_MultipleArchives(t *testing.T) {
	const numArchives = 10
	mapFS := fstest.MapFS{}
	for i := 0; i < numArchives; i++ {
		zipBytes := createZipArchive(map[string]string{
			"content.txt": fmt.Sprintf("content for archive %d", i),
		})
		mapFS[fmt.Sprintf("arch_%d.zip", i)] = &fstest.MapFile{
			Data: zipBytes,
			Mode: 0644,
		}
	}

	base := newCountingFS(mapFS)
	logger := createTestLogger()
	fsys, err := zipasfolder.NewFS(base, 20, time.Minute, true, logger)
	require.NoError(t, err)
	defer fsys.Close()

	const workersPerArchive = 10
	var wg sync.WaitGroup
	wg.Add(numArchives * workersPerArchive)

	startBarrier := make(chan struct{})

	for i := 0; i < numArchives; i++ {
		for w := 0; w < workersPerArchive; w++ {
			archiveIdx := i
			go func() {
				defer wg.Done()
				<-startBarrier

				path := fmt.Sprintf("arch_%d.zip/content.txt", archiveIdx)
				expected := fmt.Sprintf("content for archive %d", archiveIdx)

				data, err := fsys.ReadFile(path)
				assert.NoError(t, err)
				assert.Equal(t, expected, string(data))
			}()
		}
	}

	close(startBarrier)
	wg.Wait()
}

func TestCacheEvictionUnderPressure(t *testing.T) {
	const numArchives = 20
	mapFS := fstest.MapFS{}
	for i := 0; i < numArchives; i++ {
		zipBytes := createZipArchive(map[string]string{
			"test.txt": fmt.Sprintf("data in %d", i),
		})
		mapFS[fmt.Sprintf("pressure_%d.zip", i)] = &fstest.MapFile{
			Data: zipBytes,
			Mode: 0644,
		}
	}

	logger := createTestLogger()
	// Set small cache size = 3 to trigger heavy cache eviction
	fsys, err := zipasfolder.NewFS(mapFS, 3, 50*time.Millisecond, true, logger)
	require.NoError(t, err)
	defer fsys.Close()

	var wg sync.WaitGroup
	for iter := 0; iter < 5; iter++ {
		for i := 0; i < numArchives; i++ {
			wg.Add(1)
			archID := i
			go func() {
				defer wg.Done()
				p := fmt.Sprintf("pressure_%d.zip/test.txt", archID)
				data, err := fsys.ReadFile(p)
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("data in %d", archID), string(data))
			}()
		}
	}
	wg.Wait()

	// Manually trigger ClearUnlocked to ensure eviction queues drain safely
	err = fsys.ClearUnlocked()
	assert.NoError(t, err)
}

func TestDoubleCloseAndIdempotency(t *testing.T) {
	zipData := createZipArchive(map[string]string{
		"sample.txt": "sample content",
	})
	mapFS := fstest.MapFS{
		"test.zip": &fstest.MapFile{Data: zipData, Mode: 0644},
	}

	logger := createTestLogger()
	fsys, err := zipasfolder.NewFS(mapFS, 10, time.Minute, false, logger)
	require.NoError(t, err)

	f, err := fsys.Open("test.zip/sample.txt")
	require.NoError(t, err)

	buf := make([]byte, 64)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected read error: %v", err)
	}
	assert.Equal(t, "sample content", string(buf[:n]))

	// Repeated closes on the file handle must be idempotent and not corrupt ref count
	for i := 0; i < 5; i++ {
		err := f.Close()
		assert.NoError(t, err)
	}

	// Repeated closes on the filesystem itself must be safe
	for i := 0; i < 3; i++ {
		err := fsys.Close()
		assert.NoError(t, err)
	}
}

func TestZipEscape_HashZip(t *testing.T) {
	zipData := createZipArchive(map[string]string{
		"nested.txt": "nested zip content",
	})
	mapFS := fstest.MapFS{
		"dir/myarchive.zip": &fstest.MapFile{Data: zipData, Mode: 0644},
	}

	logger := createTestLogger()
	fsys, err := zipasfolder.NewFS(mapFS, 10, time.Minute, false, logger)
	require.NoError(t, err)
	defer fsys.Close()

	// 1. Reading as folder: dir/myarchive.zip/nested.txt
	data, err := fsys.ReadFile("dir/myarchive.zip/nested.txt")
	require.NoError(t, err)
	assert.Equal(t, "nested zip content", string(data))

	// 2. Reading raw zip file via #zip escape: dir/myarchive.zip#zip
	rawZip, err := fsys.ReadFile("dir/myarchive.zip#zip")
	require.NoError(t, err)
	assert.Equal(t, zipData, rawZip)

	// 3. Stat on #zip
	info, err := fsys.Stat("dir/myarchive.zip#zip")
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Equal(t, int64(len(zipData)), info.Size())

	// 4. Stat on folder-expanded zip: dir/myarchive.zip
	dirInfo, err := fsys.Stat("dir/myarchive.zip")
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
}

func TestSubFS(t *testing.T) {
	zipData := createZipArchive(map[string]string{
		"folder/fileA.txt": "hello A",
		"folder/fileB.txt": "hello B",
	})
	mapFS := fstest.MapFS{
		"archives/data.zip": &fstest.MapFile{Data: zipData, Mode: 0644},
	}

	logger := createTestLogger()
	fsys, err := zipasfolder.NewFS(mapFS, 10, time.Minute, false, logger)
	require.NoError(t, err)
	defer fsys.Close()

	subFS, err := fsys.Sub("archives/data.zip/folder")
	require.NoError(t, err)

	dataA, err := fs.ReadFile(subFS, "fileA.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello A", string(dataA))

	dataB, err := fs.ReadFile(subFS, "fileB.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello B", string(dataB))

	entries, err := fs.ReadDir(subFS, ".")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestReadDir(t *testing.T) {
	zipData := createZipArchive(map[string]string{
		"sub/deep.txt": "deep data",
		"root.txt":     "root data",
	})
	mapFS := fstest.MapFS{
		"docs/readme.txt":    &fstest.MapFile{Data: []byte("readme"), Mode: 0644},
		"docs/archive.zip":   &fstest.MapFile{Data: zipData, Mode: 0644},
		"docs/other.zip#zip": &fstest.MapFile{Data: zipData, Mode: 0644},
	}

	logger := createTestLogger()
	fsys, err := zipasfolder.NewFS(mapFS, 10, time.Minute, false, logger)
	require.NoError(t, err)
	defer fsys.Close()

	// ReadDir on "docs" should show archive.zip as a directory
	entries, err := fsys.ReadDir("docs")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	foundZip := false
	for _, e := range entries {
		if e.Name() == "archive.zip" {
			foundZip = true
			assert.True(t, e.IsDir(), "archive.zip should be reported as IsDir()=true")
		}
	}
	assert.True(t, foundZip, "expected archive.zip in ReadDir results")

	// ReadDir inside archive
	zipEntries, err := fsys.ReadDir("docs/archive.zip")
	require.NoError(t, err)
	assert.NotEmpty(t, zipEntries)
}
