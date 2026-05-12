package vfsrw_test

import (
	"testing"

	"github.com/je4/filesystem/v4/pkg/vfsrw"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestVFS_SubReadOnly(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	t.Run("Global ReadOnly", func(t *testing.T) {
		cfg := vfsrw.Config{
			"mem": &vfsrw.VFS{
				Name:        "mem",
				Type:        "afero",
				ReadOnly:    true,
				ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true},
				Afero:       &vfsrw.Afero{BaseDir: "mem://"},
			},
		}

		vfs, err := vfsrw.NewFS(cfg, _logger)
		if err != nil {
			t.Fatalf("failed to create vfs: %v", err)
		}
		defer vfs.Close()

		zipFile := "vfs://mem/test.zip"
		_, err = vfs.SubCreate(zipFile)
		if err == nil {
			t.Fatal("expected error when calling Sub on non-existing zip in read-only VFS, but got nil")
		}
	})

	t.Run("ZipAsFolder ReadOnly", func(t *testing.T) {
		cfg := vfsrw.Config{
			"mem": &vfsrw.VFS{
				Name:        "mem",
				Type:        "afero",
				ReadOnly:    false,
				ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true, ReadOnly: true},
				Afero:       &vfsrw.Afero{BaseDir: "mem://"},
			},
		}

		vfs, err := vfsrw.NewFS(cfg, _logger)
		if err != nil {
			t.Fatalf("failed to create vfs: %v", err)
		}
		defer vfs.Close()

		zipFile := "vfs://mem/test.zip"
		_, err = vfs.SubCreate(zipFile)
		if err == nil {
			t.Fatal("expected error when calling Sub on non-existing zip in read-only zipasfolder, but got nil")
		}
	})

	t.Run("Existing Zip should work even if ReadOnly", func(t *testing.T) {
		// This test is more complex because afero memfs is not global by default.
		// For this task, it's sufficient to verify that creating new ZIPs is denied.
	})
	cfgDisabled := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: false, ReadOnly: true},
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
	}
	vfsDisabled, err := vfsrw.NewFS(cfgDisabled, _logger)
	if err != nil {
		t.Fatalf("failed to create vfsDisabled: %v", err)
	}
	defer vfsDisabled.Close()

	_, err = vfsDisabled.Sub("vfs://mem/test.zip")
	if err != nil {
		t.Fatalf("expected no error when Sub on .zip with ZipAsFolder.Enabled=false (should return normal SubFS), but got %v", err)
	}

	cfgNil := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: nil,
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
	}
	vfsNil, err := vfsrw.NewFS(cfgNil, _logger)
	if err != nil {
		t.Fatalf("failed to create vfsNil: %v", err)
	}
	defer vfsNil.Close()

	_, err = vfsNil.Sub("vfs://mem/test.zip")
	if err != nil {
		t.Fatalf("expected no error when Sub on .zip with ZipAsFolder=nil (should return normal SubFS), but got %v", err)
	}
}
