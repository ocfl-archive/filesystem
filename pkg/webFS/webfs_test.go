package webFS

import (
	"io/fs"
	"os"
	"testing"

	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/rs/zerolog"
)

func TestWebFS_ReadFile(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	var _logger zLogger.ZLogger = &logger
	// Wrap memFS with webFS (replace with actual constructor if needed)
	wfs, err := NewFS(
		"https://upload.wikimedia.org/%%PATH%%", // wikipedia/commons/6/65/Zernez%2C_Unterengadin%2C_Graub%C3%BCnden._20-09-2023._%28actm.%29_71.jpg
		nil,                                     // No headers for this test
		false,                                   // Insecure TLS skip verify
		_logger,                                 // No logger for this test
	)
	if err != nil {
		t.Fatal(err)
	}

	// Try to read the file
	data, err := fs.ReadFile(wfs, "wikipedia/commons/6/65/Zernez%2C_Unterengadin%2C_Graub%C3%BCnden._20-09-2023._%28actm.%29_71.jpg")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("failed to read file")
	}
}
