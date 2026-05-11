package vfsrw_test

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"testing"

	"github.com/je4/filesystem/v4/pkg/vfsrw"
	"github.com/je4/filesystem/v4/pkg/writefs"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestVFS_SubZip(t *testing.T) {
	var _logger zLogger.ZLogger = new(zerolog.New(zerolog.NewConsoleWriter()))

	// 1. VFS mit MemFS konfigurieren.
	// WICHTIG: ZipAsFolder auf dem FS selbst ist hier NICHT nötig,
	// da wir vfsrw.Sub testen wollen, welches die ZIP-Logik triggert,
	// wenn die Config es erlaubt.
	cfg := vfsrw.Config{
		"mem": &vfsrw.VFS{
			Name:        "mem",
			Type:        "afero",
			ZipAsFolder: &vfsrw.ZipAsFolder{Enabled: true}, // Erlaubt vfsrw.Sub ZIP-Handling zu machen
			Afero:       &vfsrw.Afero{BaseDir: "mem://"},
		},
	}

	vfs, err := vfsrw.NewFS(cfg, _logger)
	if err != nil {
		t.Fatalf("failed to create vfs: %v", err)
	}
	defer vfs.Close()

	// 2. ZIP Datei Pfad definieren
	zipFile := "vfs://mem/test.zip"

	// 3. vfs.Sub aufrufen. Da test.zip noch nicht existiert, sollte vfsrw.Sub
	// ein neues zipfsrw (Write-Only/Create) erstellen, wenn ZipAsFolder aktiv ist.
	subFS, err := vfs.SubCreate(zipFile)
	if err != nil {
		t.Fatalf("failed to call vfs.Sub('%s'): %v", zipFile, err)
	}
	if subFS == nil {
		t.Fatalf("vfs.Sub('%s') returned nil, nil", zipFile)
	}

	// 4. Testdaten schreiben im Sub-Filesystem
	testFileName := "hello.txt"
	testContent := []byte("hello from sub zip")

	// Wir verwenden writefs.WriteFile um sicherzustellen, dass wir Schreiboperationen nutzen
	n, err := writefs.WriteFile(subFS, testFileName, testContent)
	if err != nil {
		t.Fatalf("failed to write file in sub zip: %v", err)
	}
	if n != int64(len(testContent)) {
		t.Fatalf("wrote %d bytes, expected %d", n, len(testContent))
	}

	// 5. ZIP schliessen (falls es ein Closer ist), um sicherzustellen, dass alles auf das zugrunde liegende FS geschrieben wird
	if closer, ok := subFS.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("failed to close sub zip fs: %v", err)
		}
	} else {
		t.Log("subFS does not implement io.Closer")
	}

	// 6. Verifizieren, dass die ZIP-Datei im Haupt-VFS existiert
	if _, err := fs.Stat(vfs, zipFile); err != nil {
		t.Fatalf("zip file does not exist after write: %v", err)
	}

	// 7. Jetzt die ZIP-Datei wieder öffnen (Read-Only) und Inhalt prüfen
	// Da vfsrw.Sub nun zipfsw (write-only) verwendet, können wir nicht mehr über Sub lesen.
	// Wir lesen die ZIP-Datei als Ganzes und verwenden archive/zip zur Verifizierung.
	t.Log("Verifying ZIP content manually since vfsrw.Sub is now write-only")

	zipData, err := fs.ReadFile(vfs, zipFile)
	if err != nil {
		t.Fatalf("failed to read zip file: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}

	found := false
	for _, f := range zr.File {
		if f.Name == testFileName {
			found = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open file in zip: %v", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("failed to read file in zip: %v", err)
			}
			if string(content) != string(testContent) {
				t.Fatalf("content mismatch: expected '%s', got '%s'", string(testContent), string(content))
			}
		}
	}
	if !found {
		t.Fatalf("file '%s' not found in zip", testFileName)
	}
}
