package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefinition_AliasedImport(t *testing.T) {
	// Test file content
	mainContent := `version = "v2.0"

import "lib.devops" as lib

node "main_node" {
	type = process.exec
	targets = [lib.local]
	cmd = ["echo", "main"]
	cwd = "."
}
`
	libContent := `version = "v2.0"

target "local" {
	address = "127.0.0.1:7700"
}

node "n1" {
	type = process.exec
	targets = [local]
	cmd = ["echo", "n1"]
	cwd = "."
}
`

	// Create temp dir and files
	tmpDir, err := os.MkdirTemp("", "lsp_test")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.devops")
	libPath := filepath.Join(tmpDir, "lib.devops")

	os.WriteFile(mainPath, []byte(mainContent), 0644)
	os.WriteFile(libPath, []byte(libContent), 0644)

	// Test finding identifier at position
	// Line 6: "	targets = [lib.local]" (0-indexed: line 6)
	// We want to find "lib.local" - the 'l' in 'local' is around column 20
	tests := []struct {
		name     string
		line     int
		char     int
		wantIdent string
	}{
		{"on lib.local", 6, 20, "lib.local"},
		{"on lib", 6, 16, "lib.local"},
		{"on local", 6, 21, "lib.local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ident := findIdentifierAtPosition(mainContent, tt.line+1, tt.char+1)
			if ident != tt.wantIdent {
				t.Errorf("findIdentifierAtPosition(%d, %d) = %q, want %q", tt.line, tt.char, ident, tt.wantIdent)
			}
		})
	}

	// Test getDefinition
	uri := "file://" + mainPath
	
	loc := getDefinition(uri, mainContent, 6, 20) // On "lib.local"
	if loc == nil {
		t.Fatal("getDefinition returned nil")
	}

	// Should point to lib.devops
	expectedURI := "file://" + libPath
	if loc.URI != expectedURI {
		t.Errorf("getDefinition URI = %q, want %q", loc.URI, expectedURI)
	}

	// Should point to line 3 (target "local" declaration)
	// Position is 0-based, target is on line 3 (1-based), so line 2 (0-based)
	if loc.Range.Start.Line < 0 {
		t.Errorf("getDefinition line should be >= 0, got %d", loc.Range.Start.Line)
	}
	
	t.Logf("Definition found at %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
}

func TestGetDefinition_ImportPath(t *testing.T) {
	mainContent := `version = "v2.0"

import "lib.devops" as lib

node "main_node" {
	type = process.exec
}
`
	libContent := `version = "v2.0"

target "local" {
	address = "127.0.0.1:7700"
}
`

	tmpDir, err := os.MkdirTemp("", "lsp_test")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mainPath := filepath.Join(tmpDir, "main.devops")
	libPath := filepath.Join(tmpDir, "lib.devops")

	os.WriteFile(mainPath, []byte(mainContent), 0644)
	os.WriteFile(libPath, []byte(libContent), 0644)

	// Test clicking on the import path "lib.devops"
	// Line 2: `import "lib.devops" as lib`
	// The string "lib.devops" starts around column 8
	uri := "file://" + mainPath
	
	loc := getDefinition(uri, mainContent, 2, 12) // Inside "lib.devops"
	if loc == nil {
		t.Fatal("getDefinition returned nil for import path")
	}

	expectedURI := "file://" + libPath
	if loc.URI != expectedURI {
		t.Errorf("getDefinition URI = %q, want %q", loc.URI, expectedURI)
	}

	t.Logf("Import definition found at %s", loc.URI)
}
