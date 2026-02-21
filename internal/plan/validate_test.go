package plan_test

import (
	"devopsctl/internal/plan"
	"strings"
	"testing"
)

func TestValidateProcessExecValid(t *testing.T) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "t1", Address: "1.2.3.4:7700"}},
		Nodes: []plan.Node{{
			ID:      "n1",
			Type:    "process.exec",
			Targets: []string{"t1"},
			Inputs: map[string]any{
				"cmd": []any{"ls", "-la"},
				"cwd": "/tmp",
			},
		}},
	}
	errs := plan.Validate(p)
	if len(errs) != 0 {
		t.Fatalf("expected valid process.exec node, got errors: %v", errs)
	}
}

func TestValidateProcessExecMissingCmd(t *testing.T) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "t1", Address: "1.2.3.4:7700"}},
		Nodes: []plan.Node{{
			ID:      "n1",
			Type:    "process.exec",
			Targets: []string{"t1"},
			Inputs: map[string]any{
				"cwd": "/tmp",
			},
		}},
	}
	errs := plan.Validate(p)
	if len(errs) == 0 {
		t.Fatal("expected error for missing cmd in process.exec")
	}
	if !strings.Contains(errs[0].Error(), "requires non-empty array 'cmd'") {
		t.Errorf("unexpected error message: %v", errs[0])
	}
}

func TestValidateProcessExecEmptyCmd(t *testing.T) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "t1", Address: "1.2.3.4:7700"}},
		Nodes: []plan.Node{{
			ID:      "n1",
			Type:    "process.exec",
			Targets: []string{"t1"},
			Inputs: map[string]any{
				"cmd": []any{},
				"cwd": "/tmp",
			},
		}},
	}
	errs := plan.Validate(p)
	if len(errs) == 0 {
		t.Fatal("expected error for empty cmd in process.exec")
	}
	if !strings.Contains(errs[0].Error(), "requires non-empty array 'cmd'") {
		t.Errorf("unexpected error message: %v", errs[0])
	}
}

func TestValidateProcessExecMissingCwd(t *testing.T) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "t1", Address: "1.2.3.4:7700"}},
		Nodes: []plan.Node{{
			ID:      "n1",
			Type:    "process.exec",
			Targets: []string{"t1"},
			Inputs: map[string]any{
				"cmd": []any{"echo", "hello"},
			},
		}},
	}
	errs := plan.Validate(p)
	if len(errs) == 0 {
		t.Fatal("expected error for missing cwd in process.exec")
	}
	if !strings.Contains(errs[0].Error(), "requires string 'cwd'") {
		t.Errorf("unexpected error message: %v", errs[0])
	}
}
