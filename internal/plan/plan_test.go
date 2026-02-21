package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"devopsctl/internal/plan"
)

const validPlan = `{
  "version": "1.0",
  "targets": [{"id": "web-1", "address": "127.0.0.1:7700"}],
  "nodes": [{
    "id": "file.app",
    "type": "file.sync",
    "targets": ["web-1"],
    "inputs": {"src": "./build", "dest": "/var/www/app"}
  }]
}`

func TestLoadAndValidate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "plan.json")
	if err := os.WriteFile(path, []byte(validPlan), 0644); err != nil {
		t.Fatal(err)
	}
	p, _, err := plan.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	errs := plan.Validate(p)
	if len(errs) != 0 {
		t.Fatalf("Validate returned errors: %v", errs)
	}
}

func TestValidateMissingFields(t *testing.T) {
	p := &plan.Plan{} // all empty
	errs := plan.Validate(p)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for empty plan")
	}
}

func TestValidateUnknownTarget(t *testing.T) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "t1", Address: "1.2.3.4:7700"}},
		Nodes: []plan.Node{{
			ID:      "n1",
			Type:    "file.sync",
			Targets: []string{"t-unknown"},
			Inputs:  map[string]any{"src": ".", "dest": "/tmp/x"},
		}},
	}
	errs := plan.Validate(p)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown target reference")
	}
}
