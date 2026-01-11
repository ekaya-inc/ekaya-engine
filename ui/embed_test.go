package ui

import (
	"io/fs"
	"strings"
	"testing"
)

// TestDistFSEmbedded verifies that the UI dist directory is properly embedded
func TestDistFSEmbedded(t *testing.T) {
	// Test that we can access the dist subdirectory
	distFS, err := fs.Sub(DistFS, "dist")
	if err != nil {
		t.Fatalf("Failed to access dist subdirectory: %v", err)
	}

	// Test that index.html exists in the embedded filesystem
	indexData, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		t.Fatalf("Failed to read index.html from embedded filesystem: %v", err)
	}

	if len(indexData) == 0 {
		t.Fatal("index.html is empty")
	}

	// Verify it looks like HTML (basic sanity check)
	content := string(indexData)
	if len(content) < 100 {
		t.Errorf("index.html seems too short (%d bytes), might be invalid", len(content))
	}

	// Check for typical HTML markers
	if !strings.Contains(content, "<!DOCTYPE") && !strings.Contains(content, "<html") {
		t.Error("index.html does not appear to be valid HTML (missing DOCTYPE or <html>)")
	}
}

// TestAssetsDirectoryEmbedded verifies that the assets subdirectory is embedded
func TestAssetsDirectoryEmbedded(t *testing.T) {
	distFS, err := fs.Sub(DistFS, "dist")
	if err != nil {
		t.Fatalf("Failed to access dist subdirectory: %v", err)
	}

	// Check if assets directory exists
	entries, err := fs.ReadDir(distFS, "assets")
	if err != nil {
		t.Fatalf("Failed to read assets directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("assets directory is empty, expected at least some compiled assets")
	}

	// Verify we can read at least one file from assets
	foundReadableFile := false
	for _, entry := range entries {
		if !entry.IsDir() {
			data, err := fs.ReadFile(distFS, "assets/"+entry.Name())
			if err == nil && len(data) > 0 {
				foundReadableFile = true
				break
			}
		}
	}

	if !foundReadableFile {
		t.Error("Could not read any files from assets directory")
	}
}
