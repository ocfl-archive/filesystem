package zipfsrw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ocfl-archive/filesystem/pkg/osfsrw"
	"github.com/ocfl-archive/filesystem/pkg/writefs"
	"github.com/rs/zerolog"
)

var testTmpFile = "file://" + filepath.ToSlash(filepath.Join(os.TempDir(), tempFileName("zipfsrwfactorytest_", ".zip")))
var factory *writefs.Factory

func TestZipFSRWFactory(t *testing.T) {
	var err error
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	factory, err = writefs.NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	if err := factory.Register(osfsrw.NewCreateFSFunc(&logger), "^file://", writefs.MediumFS); err != nil {
		t.Fatal(err)
	}
	if err := factory.Register(NewCreateFSFunc(false, &logger), "\\.zip$", writefs.HighFS); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
	})

	t.Run("create", testZipFSRWFactory_create)
}

func testZipFSRWFactory_create(t *testing.T) {
	fs, err := factory.Get(testTmpFile, false)
	if err != nil {
		t.Fatal(err)
	}
	if fs == nil {
		t.Fatal("fs is nil")
	}
}
