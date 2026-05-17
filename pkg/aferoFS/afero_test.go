package aferoFS

import (
	"os"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestAferoFS(t *testing.T) {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	var logger zLogger.ZLogger = new(zerolog.New(out).With().Timestamp().Logger())

	t.Run("MemMapFS", func(t *testing.T) {
		memFs := afero.NewMemMapFs()
		fsys, err := NewFS(memFs, logger)
		assert.NoError(t, err)

		filename := "test.txt"
		content := []byte("hello world")

		// Test WriteFile
		n, err := fsys.WriteFile(filename, content)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(content)), n)

		// Test Stat
		fi, err := fsys.Stat(filename)
		assert.NoError(t, err)
		assert.Equal(t, filename, fi.Name())

		// Test ReadFile
		data, err := fsys.ReadFile(filename)
		assert.NoError(t, err)
		assert.Equal(t, content, data)

		// Test Open
		f, err := fsys.Open(filename)
		assert.NoError(t, err)
		defer f.Close()

		stat, err := f.Stat()
		assert.NoError(t, err)
		assert.Equal(t, filename, stat.Name())

		// Test ReadDir
		err = fsys.MkDir("subdir")
		assert.NoError(t, err)

		entries, err := fsys.ReadDir(".")
		assert.NoError(t, err)
		assert.Len(t, entries, 2) // test.txt and subdir
	})

	t.Run("NewCreateFSFunc complex schemes", func(t *testing.T) {
		factory, _ := writefs.NewFactory()
		createFunc := NewCreateFSFunc(logger)

		// Register the createFunc for various patterns
		err := factory.Register(createFunc, "^mem://", writefs.LowFS)
		assert.NoError(t, err)
		err = factory.Register(createFunc, "^ro://", writefs.LowFS)
		assert.NoError(t, err)
		err = factory.Register(createFunc, "^cow://", writefs.LowFS)
		assert.NoError(t, err)

		// 1. Create a memory FS
		_, err = factory.Get("mem://mem1", false)
		assert.NoError(t, err)

		// 2. Create a readonly FS pointing to the memory FS
		roFS, err := factory.Get("ro://?base=mem://mem1", false)
		if assert.NoError(t, err) {
			_, err = writefs.WriteFile(roFS, "test.txt", []byte("should fail"))
			assert.Error(t, err, "Writing to readonly FS should fail")
		}

		// 3. Create a CopyOnWrite FS
		cowFS, err := factory.Get("cow://?base=ro://?base=mem://mem1&layer=mem://mem2", false)
		if assert.NoError(t, err) {
			testData := []byte("cow data")
			_, err = writefs.WriteFile(cowFS, "cow_test.txt", testData)
			assert.NoError(t, err, "Writing to COW FS should succeed")

			data, err := afero.ReadFile(cowFS.(interface{ GetAfero() afero.Fs }).GetAfero(), "cow_test.txt")
			assert.NoError(t, err)
			assert.Equal(t, testData, data)
		}
	})
}
