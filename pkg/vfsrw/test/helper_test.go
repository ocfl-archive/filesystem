package vfsrw_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/je4/filesystem/v4/pkg/vfsrw"
)

func TestMatchPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get current working directory: %v", err)
	}
	wd = filepath.ToSlash(wd)
	currentDrive := strings.ToLower(wd[0:1])

	tests := []struct {
		name     string
		input    string
		wantName string
		wantPath string
		wantErr  bool
		windows  bool
	}{
		{
			name:     "vfs standard",
			input:    "vfs://c/test/file.txt",
			wantName: "c",
			wantPath: "test/file.txt",
		},
		{
			name:     "vfs short",
			input:    "vfs://data",
			wantName: "data",
			wantPath: "",
		},
		{
			name:     "vfs with slash",
			input:    "vfs://data/",
			wantName: "data",
			wantPath: "",
		},
		{
			name:     "absolute windows path with drive",
			input:    `C:\temp\test.txt`,
			wantName: "c",
			wantPath: "temp/test.txt",
			windows:  true,
		},
		{
			name:     "absolute windows path lowercase drive",
			input:    `d:/data/info.log`,
			wantName: "d",
			wantPath: "data/info.log",
			windows:  true,
		},
		{
			name:     "absolute windows path without drive",
			input:    `\temp\test.txt`,
			wantName: currentDrive,
			wantPath: "temp/test.txt",
			windows:  true,
		},
		{
			name:     "relative path",
			input:    `subdir\file.txt`,
			wantName: currentDrive,
			wantPath: strings.TrimPrefix(filepath.ToSlash(filepath.Join(wd[2:], "subdir", "file.txt")), "/"),
			windows:  true,
		},
		{
			name:     "current dir relative",
			input:    `.`,
			wantName: currentDrive,
			wantPath: strings.TrimPrefix(wd[2:], "/"),
			windows:  true,
		},
		{
			name:     "absolute linux path",
			input:    "/etc/passwd",
			wantName: "root",
			wantPath: "etc/passwd",
			windows:  false,
		},
		{
			name:     "relative linux path",
			input:    "subdir/file.txt",
			wantName: "root",
			wantPath: strings.TrimPrefix(filepath.ToSlash(filepath.Join(wd, "subdir", "file.txt")), "/"),
			windows:  false,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid vfs uri",
			input:   "vfs://",
			wantErr: true,
		},
	}

	isWindows := os.PathSeparator == '\\'

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windows && !isWindows {
				t.Skip("Skipping Windows-specific test on non-Windows platform")
			}
			if !tt.windows && isWindows && tt.name != "vfs standard" && tt.name != "vfs short" && tt.name != "vfs with slash" {
				// "vfs standard" etc. sind plattformunabhängig, aber ich habe sie nicht explizit markiert.
				// Wir skippen nur die Linux-spezifischen auf Windows.
				if strings.Contains(tt.name, "linux") {
					t.Skip("Skipping Linux-specific test on Windows")
				}
			}
			gotName, gotPath, err := vfsrw.MatchPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("matchPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotName != tt.wantName {
				t.Errorf("matchPath() gotName = %v, want %v", gotName, tt.wantName)
			}
			if gotPath != tt.wantPath {
				t.Errorf("matchPath() gotPath = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}
