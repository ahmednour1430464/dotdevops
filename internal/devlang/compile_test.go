package devlang

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"devopsctl/internal/plan"
)

func mustParseAndValidate(t *testing.T, path string, src string) *File {
	t.Helper()
	file, parseErrs := ParseFile(path, []byte(src))
	if len(parseErrs) > 0 {
		for _, e := range parseErrs {
			t.Logf("parse error: %v", e)
		}
		t.Fatalf("unexpected parse errors (%d)", len(parseErrs))
	}
	return file
}

func TestCompile_BaselineFileSyncAndExec(t *testing.T) {
	src := `
		target "local" {
		  address = "127.0.0.1:7700"
		}

		node "test-sync" {
		  type    = file.sync
		  targets = [local]

		  src  = "./testsrc"
		  dest = "/tmp/testdest"
		}

		node "test-exec" {
		  type    = process.exec
		  targets = [local]

		  cmd = ["go", "version"]
		  cwd = "/tmp"
		}
	`

	res, err := CompileFileV0_1("test.devops", []byte(src))
	if err != nil {
		t.Fatalf("CompileFileV0_1 error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected semantic errors: %v", res.Errors)
	}

	want := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "local", Address: "127.0.0.1:7700"}},
		Nodes: []plan.Node{
			{
				ID:      "test-sync",
				Type:    "file.sync",
				Targets: []string{"local"},
				Inputs: map[string]any{
					"src":  "./testsrc",
					"dest": "/tmp/testdest",
				},
			},
			{
				ID:      "test-exec",
				Type:    "process.exec",
				Targets: []string{"local"},
				Inputs: map[string]any{
					"cmd": []any{"go", "version"},
					"cwd": "/tmp",
				},
			},
		},
	}

	if !reflect.DeepEqual(res.Plan, want) {
		gotJSON, _ := json.MarshalIndent(res.Plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("plan mismatch.\nGot:\n%s\nWant:\n%s", string(gotJSON), string(wantJSON))
	}
}

func TestCompile_PlanResumeDevopsMatchesJSON(t *testing.T) {
	jsonData, err := os.ReadFile("../../tests/e2e/plan_resume.json")
	if err != nil {
		t.Fatalf("read plan_resume.json: %v", err)
	}
	var want plan.Plan
	if err := json.Unmarshal(jsonData, &want); err != nil {
		t.Fatalf("unmarshal plan_resume.json: %v", err)
	}

	devopsData, err := os.ReadFile("../../tests/e2e/plan_resume.devops")
	if err != nil {
		t.Fatalf("read plan_resume.devops: %v", err)
	}

	res, err := CompileFileV0_1("plan_resume.devops", devopsData)
	if err != nil {
		t.Fatalf("CompileFileV0_1 error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected semantic errors: %v", res.Errors)
	}

	if !reflect.DeepEqual(res.Plan, &want) {
		gotJSON, _ := json.MarshalIndent(res.Plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(&want, "", "  ")
		t.Fatalf("plan mismatch.\nGot:\n%s\nWant:\n%s", string(gotJSON), string(wantJSON))
	}
}

func TestValidateV0_1_UnknownTarget(t *testing.T) {
	src := `
		target "local" {
		  address = "127.0.0.1:7700"
		}

		node "file.app" {
		  type    = file.sync
		  targets = [local, prod]

		  src  = "./src"
		  dest = "/tmp/dest"
		}
	`

	file := mustParseAndValidate(t, "unknown_target.devops", src)
	errs := ValidateV0_1(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic error for unknown target, got none")
	}
	if !containsErrorMessage(errs, "unknown target \"prod\"") {
		t.Fatalf("expected error containing 'unknown target \"prod\"', got: %v", errs)
	}
}

func TestValidateV0_1_DuplicateNode(t *testing.T) {
	src := `
		target "local" { address = "127.0.0.1:7700" }

		node "dup" {
		  type    = file.sync
		  targets = [local]
		  src  = "./a"
		  dest = "/tmp/a"
		}

		node "dup" {
		  type    = file.sync
		  targets = [local]
		  src  = "./b"
		  dest = "/tmp/b"
		}
	`

	file := mustParseAndValidate(t, "dup_node.devops", src)
	errs := ValidateV0_1(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic error for duplicate node, got none")
	}
	if !containsErrorMessage(errs, "duplicate node \"dup\"") {
		t.Fatalf("expected duplicate node error, got: %v", errs)
	}
}

func TestValidateV0_1_InvalidFailurePolicy(t *testing.T) {
	src := `
		target "local" { address = "127.0.0.1:7700" }

		node "n" {
		  type           = process.exec
		  targets        = [local]
		  failure_policy = fast

		  cmd = ["echo", "hi"]
		  cwd = "/tmp"
		}
	`

	file := mustParseAndValidate(t, "bad_policy.devops", src)
	errs := ValidateV0_1(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic error for invalid failure_policy, got none")
	}
	if !containsErrorMessage(errs, "invalid failure_policy \"fast\"") {
		t.Fatalf("expected invalid failure_policy error, got: %v", errs)
	}
}

func TestValidateV0_1_UnsupportedLet(t *testing.T) {
	src := `
		let x = "foo"
	`

	file := mustParseAndValidate(t, "let_unsupported.devops", src)
	errs := ValidateV0_1(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic error for unsupported let, got none")
	}
	if !containsErrorMessage(errs, "let bindings are not supported in language version 0.1") {
		t.Fatalf("expected unsupported let error, got: %v", errs)
	}
}

func containsErrorMessage(errs []error, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), substr) {
			return true
		}
	}
	return false
}
