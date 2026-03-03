package lsp

import (
	"testing"
)

func TestPrimitiveTypeCompletions(t *testing.T) {
	// Test: type = completion inside node
	text := `node "test" {
    type = 
}`
	// Line 1, character 10 (after "type = ")
	items := GetCompletions(text, 1, 10)

	// Should show primitive types
	if len(items) == 0 {
		t.Error("Expected primitive type completions, got none")
	}

	// Check that we have some expected primitives
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Label] = true
	}

	expectedTypes := []string{"process.exec", "file.sync", "_fs.write", "_exec"}
	for _, expected := range expectedTypes {
		if !found[expected] {
			t.Errorf("Expected to find primitive type %q in completions", expected)
		}
	}
}

func TestAliasDotCompletion(t *testing.T) {
	// Test: alias. completion inside targets
	text := `import "lib.devops" as lib

node "test" {
    type = process.exec
    targets = [lib.
}`
	// Line 4, character 22 (after "lib.")
	items := GetCompletions(text, 4, 22)

	// Since we don't have an actual lib.devops file, this might return empty
	// but the context should be contextAliasDot
	// For now, just check it doesn't crash
	t.Logf("Got %d completions for alias dot", len(items))
}

func TestTargetBodyCompletion(t *testing.T) {
	// Test: inside target body
	text := `target "web1" {
    
}`
	// Line 1, character 4 (inside the braces)
	items := GetCompletions(text, 1, 4)

	if len(items) == 0 {
		t.Error("Expected target field completions, got none")
	}

	// Check that we have expected fields
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Label] = true
	}

	expectedFields := []string{"address", "labels"}
	for _, expected := range expectedFields {
		if !found[expected] {
			t.Errorf("Expected to find field %q in target completions", expected)
		}
	}
}

func TestFleetBodyCompletion(t *testing.T) {
	// Test: inside fleet body
	text := `fleet "web" {
    
}`
	// Line 1, character 4 (inside the braces)
	items := GetCompletions(text, 1, 4)

	if len(items) == 0 {
		t.Error("Expected fleet field completions, got none")
	}

	// Check that we have expected fields
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Label] = true
	}

	if !found["match"] {
		t.Error("Expected to find 'match' field in fleet completions")
	}
}

func TestNodeBodyCompletion(t *testing.T) {
	// Test: inside node body
	text := `node "app" {
    type = process.exec
    
}`
	// Line 2, character 4 (inside the braces after type)
	items := GetCompletions(text, 2, 4)

	if len(items) == 0 {
		t.Error("Expected node field completions, got none")
	}

	// Check that we have expected fields
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Label] = true
	}

	expectedFields := []string{"targets", "cmd", "cwd"}
	for _, expected := range expectedFields {
		if !found[expected] {
			t.Errorf("Expected to find field %q in node completions", expected)
		}
	}
}

func TestProbeBodyCompletion(t *testing.T) {
	// Test: inside probe body
	text := `primitive "test" {
    probe {
        
    }
}`
	// Line 2, character 8 (inside probe)
	items := GetCompletions(text, 2, 8)

	if len(items) == 0 {
		t.Error("Expected probe function completions, got none")
	}

	// Check that we have expected functions
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Label] = true
	}

	if !found["_fs.exists"] {
		t.Error("Expected to find '_fs.exists' in probe completions")
	}
}

func TestDetectAliasDot(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"lib.", "lib"},
		{"[lib.", "lib"},
		{"targets = [lib.", "lib"},
		{", lib.", "lib"},
		{"    lib.", "lib"},
		{"something.else.lib.", "lib"},
		{"noalias", ""},
		{".", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := detectAliasDotAccess(tt.input)
		if result != tt.expected {
			t.Errorf("detectAliasDotAccess(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
