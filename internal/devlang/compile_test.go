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

func TestCompileV0_2_WithStringLetBinding(t *testing.T) {
	src := `
		let app_dir = "/var/www/app"

		target "local" {
		  address = "127.0.0.1:7700"
		}

		node "sync" {
		  type    = file.sync
		  targets = [local]

		  src  = "./src"
		  dest = app_dir
		}
	`

	res, err := CompileFileV0_2("let_string.devops", []byte(src))
	if err != nil {
		t.Fatalf("CompileFileV0_2 error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected semantic errors: %v", res.Errors)
	}

	want := &plan.Plan{
		Version: "1.0",
		Targets: []plan.Target{{ID: "local", Address: "127.0.0.1:7700"}},
		Nodes: []plan.Node{
			{
				ID:      "sync",
				Type:    "file.sync",
				Targets: []string{"local"},
				Inputs: map[string]any{
					"src":  "./src",
					"dest": "/var/www/app",
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

func TestCompileV0_2_WithListLetBinding(t *testing.T) {
	src := `
		let migrate_cmd = ["php", "artisan", "migrate", "--force"]

		target "local" {
		  address = "127.0.0.1:7700"
		}

		node "migrate" {
		  type    = process.exec
		  targets = [local]

		  cmd = migrate_cmd
		  cwd = "/tmp"
		}
	`

	res, err := CompileFileV0_2("let_list.devops", []byte(src))
	if err != nil {
		t.Fatalf("CompileFileV0_2 error: %v", err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected semantic errors: %v", res.Errors)
	}

	if len(res.Plan.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(res.Plan.Nodes))
	}
	node := res.Plan.Nodes[0]
	cmdVal, ok := node.Inputs["cmd"]
	if !ok {
		t.Fatalf("expected cmd input in node")
	}
	cmdSlice, ok := cmdVal.([]any)
	if !ok {
		t.Fatalf("expected cmd to be []any, got %T", cmdVal)
	}

	wantCmd := []any{"php", "artisan", "migrate", "--force"}
	if !reflect.DeepEqual(cmdSlice, wantCmd) {
		gotJSON, _ := json.MarshalIndent(cmdSlice, "", "  ")
		wantJSON, _ := json.MarshalIndent(wantCmd, "", "  ")
		t.Fatalf("cmd mismatch.\nGot:\n%s\nWant:\n%s", string(gotJSON), string(wantJSON))
	}
}

func TestValidateV0_2_DuplicateLet(t *testing.T) {
	src := `
		let x = "a"
		let x = "b"
	`

	file := mustParseAndValidate(t, "dup_let.devops", src)
	errs, _ := ValidateV0_2(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic error for duplicate let, got none")
	}
	if !containsErrorMessage(errs, "duplicate let \"x\"") {
		t.Fatalf("expected duplicate let error, got: %v", errs)
	}
}

func TestValidateV0_2_LiteralKinds(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantErrSub string
	}{
		{
			name: "string literal",
			src:  "let a = \"foo\"",
		},
		{
			name: "bool literal",
			src:  "let b = true",
		},
		{
			name: "string list literal",
			src:  "let c = [\"a\", \"b\"]",
		},
		{
			name:       "identifier value disallowed",
			src:        "let d = foo",
			wantErrSub: "let \"d\" value must be a string, bool, or list of string literals",
		},
		{
			name:       "list with identifier element disallowed",
			src:        "let e = [\"a\", foo]",
			wantErrSub: "let \"e\" value must be a string, bool, or list of string literals",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := mustParseAndValidate(t, "lits.devops", tt.src)
			errs, _ := ValidateV0_2(file)
			if tt.wantErrSub == "" {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got: %v", errs)
				}
			} else {
				if !containsErrorMessage(errs, tt.wantErrSub) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErrSub, errs)
				}
			}
		})
	}
}

func TestValidateV0_2_LetInTargetsRejected(t *testing.T) {
	src := `
		let t = "local"

		target "local" { address = "127.0.0.1:7700" }

		node "n" {
		  type    = file.sync
		  targets = [t]

		  src  = "./src"
		  dest = "/tmp/dest"
		}
	`

	file := mustParseAndValidate(t, "let_in_targets.devops", src)
	errs, _ := ValidateV0_2(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic error for let in targets, got none")
	}
	if !containsErrorMessage(errs, "let binding \"t\" cannot be used in targets") {
		t.Fatalf("expected let-in-targets error, got: %v", errs)
	}
}

func TestValidateV0_2_UnsupportedStepModule(t *testing.T) {
	src := `
		step "s" {
		  type    = file.sync
		  targets = [local]

		  src  = "./src"
		  dest = "/tmp/dest"
		}

		module m {
		  target "local" { address = "127.0.0.1:7700" }
		}
	`

	file := mustParseAndValidate(t, "unsupported_constructs_v0_2.devops", src)
	errs, _ := ValidateV0_2(file)
	if len(errs) == 0 {
		t.Fatalf("expected semantic errors for unsupported constructs, got none")
	}
	if !containsErrorMessage(errs, "steps are not supported in language version 0.2") {
		t.Fatalf("expected step unsupported error, got: %v", errs)
	}
	if !containsErrorMessage(errs, "modules are not supported in language version 0.2") {
		t.Fatalf("expected module unsupported error, got: %v", errs)
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
