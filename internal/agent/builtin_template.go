package agent

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	agentcontext "devopsctl/internal/agent/context"
	"devopsctl/internal/proto"
)

// handleTemplateRender renders a Go text/template and writes the result to a file.
//
// Inputs:
//   - template  (string)  — the template body (Go text/template syntax)
//   - vars      (map)     — variables available to the template as top-level keys
//   - dest      (string)  — destination file path to write
//   - mode      (string)  — optional octal file mode e.g. "0644" (default "0644")
//   - create_dirs (bool)  — if true, create parent directories automatically
func handleTemplateRender(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	tmplBody, _ := inputs["template"].(string)
	if tmplBody == "" {
		return proto.Result{Status: "failed", Message: "template.render: 'template' input is required"}
	}

	dest, _ := inputs["dest"].(string)
	if dest == "" {
		return proto.Result{Status: "failed", Message: "template.render: 'dest' input is required"}
	}

	// Validate write access via execution context.
	if err := executor.ValidateFilePath(dest, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Message: fmt.Sprintf("template.render: %v", err)}
	}

	// Collect template variables implicitly from inputs prefixed with "var_".
	// Since devlang syntax does not support generic map literals in inputs,
	// this allows users to pass vars like: var_Port = "8080"
	vars := map[string]any{}
	for k, v := range inputs {
		if len(k) > 4 && k[:4] == "var_" {
			vars[k[4:]] = v
		}
	}

	// Parse and execute the template.
	tmpl, err := template.New("render").Option("missingkey=error").Parse(tmplBody)
	if err != nil {
		return proto.Result{Status: "failed", Message: fmt.Sprintf("template.render: parse error: %v", err)}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return proto.Result{Status: "failed", Message: fmt.Sprintf("template.render: execute error: %v", err)}
	}

	// Create parent directories if requested.
	if createDirs, _ := inputs["create_dirs"].(bool); createDirs {
		if err := os.MkdirAll(dest[:lastSlash(dest)], 0o755); err != nil {
			return proto.Result{Status: "failed", Message: fmt.Sprintf("template.render: mkdir parent: %v", err)}
		}
	}

	// Determine file mode.
	mode := os.FileMode(0o644)
	if modeStr, ok := inputs["mode"].(string); ok && modeStr != "" {
		var parsed uint32
		fmt.Sscanf(modeStr, "%o", &parsed)
		if parsed > 0 {
			mode = os.FileMode(parsed)
		}
	}

	// Write rendered content to destination.
	if err := os.WriteFile(dest, buf.Bytes(), mode); err != nil {
		return proto.Result{Status: "failed", Message: fmt.Sprintf("template.render: write %s: %v", dest, err)}
	}

	return proto.Result{
		Status:  "success",
		Message: fmt.Sprintf("rendered template to %s (%d bytes)", dest, buf.Len()),
		Stdout:  fmt.Sprintf("dest=%s bytes=%d", dest, buf.Len()),
	}
}

// lastSlash returns the index of the last '/' in s, or 0 if there is none.
func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return 0
}
