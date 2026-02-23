package context

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Executor applies context restrictions and executes commands.
type Executor struct {
	Context       *ExecutionContext // The execution context to enforce
	NodeID        string            // Node ID for audit logging
	PrimitiveType string            // Primitive type for audit logging
	AuditLogger   *AuditLogger      // Optional audit logger
}

// ExecResult contains the result of command execution.
type ExecResult struct {
	ExitCode int           // Exit code (0 = success)
	Stdout   string        // Standard output
	Stderr   string        // Standard error
	Duration time.Duration // Execution duration
}

// ExecuteCommand runs a command with context enforcement.
// It validates the command, applies restrictions, executes with user switching,
// and audits the execution.
func (e *Executor) ExecuteCommand(ctx context.Context, cmd []string, cwd string, timeout time.Duration) (*ExecResult, error) {
	startTime := time.Now()

	// 1. Validate command against context
	if err := e.validateCommand(cmd); err != nil {
		// Audit denied execution
		if e.AuditLogger != nil {
			_ = e.AuditLogger.Log(AuditEntry{
				Timestamp:     startTime,
				NodeID:        e.NodeID,
				PrimitiveType: e.PrimitiveType,
				ContextName:   e.Context.Name,
				ExecutionUser: e.Context.Identity.User,
				TrustLevel:    e.Context.TrustLevel,
				Command:       cmd,
				WorkingDir:    cwd,
				Status:        "denied",
				ErrorMessage:  err.Error(),
				Duration:      time.Since(startTime),
			})
		}
		return nil, fmt.Errorf("context validation failed: %w", err)
	}

	// 2. Build execution command with user switching
	execCmd := e.buildCommand(cmd, cwd)

	// 3. Apply timeout if specified
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 4. Execute with context
	result, err := e.runWithContext(ctx, execCmd)

	// 5. Audit execution
	e.auditExecution(cmd, cwd, result, err, startTime)

	return result, err
}

// buildCommand constructs the exec.Cmd with user switching via sudo/runuser.
func (e *Executor) buildCommand(cmd []string, cwd string) *exec.Cmd {
	if e.Context.Identity.User == "" || e.Context.Identity.User == "current" {
		// Run as current user (no user switching)
		execCmd := exec.Command(cmd[0], cmd[1:]...)
		execCmd.Dir = cwd
		return execCmd
	}

	// Build wrapper command for user switching
	var wrapperCmd []string

	if e.Context.Privilege.AllowEscalation {
		// Use sudo for privilege escalation
		wrapperCmd = []string{"sudo", "-u", e.Context.Identity.User}
		if e.Context.Privilege.NoPassword {
			wrapperCmd = append(wrapperCmd, "-n") // non-interactive
		}
	} else {
		// Use runuser for non-escalated user switching
		wrapperCmd = []string{"runuser", "-u", e.Context.Identity.User, "--"}
	}

	// Append actual command
	wrapperCmd = append(wrapperCmd, cmd...)

	execCmd := exec.Command(wrapperCmd[0], wrapperCmd[1:]...)
	execCmd.Dir = cwd
	return execCmd
}

// validateCommand checks if the command is allowed by the context.
func (e *Executor) validateCommand(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("empty command")
	}

	executable := cmd[0]

	// Check denied executables (blacklist takes precedence)
	for _, denied := range e.Context.Process.DeniedExecutables {
		if strings.Contains(executable, denied) {
			return fmt.Errorf("executable %q is denied by context %q", 
				executable, e.Context.Name)
		}
	}

	// Check allowed executables (whitelist)
	if len(e.Context.Process.AllowedExecutables) > 0 {
		allowed := false
		for _, allowedExec := range e.Context.Process.AllowedExecutables {
			if executable == allowedExec || strings.HasSuffix(executable, "/"+allowedExec) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("executable %q not in allowed list for context %q", 
				executable, e.Context.Name)
		}
	}

	return nil
}

// runWithContext executes the command and captures output.
func (e *Executor) runWithContext(ctx context.Context, cmd *exec.Cmd) (*ExecResult, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Apply environment variables from context
	if len(e.Context.Process.Environment) > 0 {
		for k, v := range e.Context.Process.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		return result, err
	}

	result.ExitCode = 0
	return result, nil
}

// auditExecution logs the execution to the audit logger if configured.
func (e *Executor) auditExecution(cmd []string, cwd string, result *ExecResult, execErr error, startTime time.Time) {
	if e.AuditLogger == nil {
		return
	}

	entry := AuditEntry{
		Timestamp:     startTime,
		NodeID:        e.NodeID,
		PrimitiveType: e.PrimitiveType,
		ContextName:   e.Context.Name,
		ExecutionUser: e.Context.Identity.User,
		TrustLevel:    e.Context.TrustLevel,
		WorkingDir:    cwd,
	}

	if result != nil {
		entry.ExitCode = result.ExitCode
		entry.Duration = result.Duration
	}

	// Log command based on audit level
	if e.Context.Audit.Level >= AuditLevelStandard {
		entry.Command = cmd
	}

	// Log outputs based on audit config
	if result != nil {
		if e.Context.Audit.LogStdout {
			entry.Stdout = result.Stdout
		}
		if e.Context.Audit.LogStderr {
			entry.Stderr = result.Stderr
		}
	}

	// Log environment for full audit
	if e.Context.Audit.Level >= AuditLevelFull && e.Context.Audit.LogEnv {
		entry.Environment = e.Context.Process.Environment
	}

	// Determine status
	if execErr != nil {
		entry.Status = "failed"
		entry.ErrorMessage = execErr.Error()
	} else if result != nil && result.ExitCode == 0 {
		entry.Status = "success"
	} else {
		entry.Status = "failed"
	}

	_ = e.AuditLogger.Log(entry)
}

// FileOperation represents a file operation type.
type FileOperation int

const (
	FileOpRead  FileOperation = iota // Read operation
	FileOpWrite                      // Write operation
)

// ValidateFilePath checks if a file path operation is allowed by the context.
func (e *Executor) ValidateFilePath(path string, operation FileOperation) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Check denied paths first (highest priority)
	for _, denied := range e.Context.Filesystem.DeniedPaths {
		if strings.HasPrefix(absPath, denied) {
			return fmt.Errorf("path %q is denied by context %q", path, e.Context.Name)
		}
	}

	// Check operation-specific permissions
	switch operation {
	case FileOpRead:
		return e.validateReadPath(absPath)
	case FileOpWrite:
		return e.validateWritePath(absPath)
	}

	return nil
}

// validateReadPath checks if a path can be read.
func (e *Executor) validateReadPath(path string) error {
	// If no restrictions, allow all
	if len(e.Context.Filesystem.ReadOnlyPaths) == 0 && len(e.Context.Filesystem.WritablePaths) == 0 {
		return nil
	}

	// Check readable paths
	for _, allowed := range e.Context.Filesystem.ReadOnlyPaths {
		if strings.HasPrefix(path, allowed) {
			return nil
		}
	}

	// Also check writable paths (can read what you can write)
	for _, allowed := range e.Context.Filesystem.WritablePaths {
		if strings.HasPrefix(path, allowed) {
			return nil
		}
	}

	return fmt.Errorf("path %q not readable in context %q", path, e.Context.Name)
}

// validateWritePath checks if a path can be written.
func (e *Executor) validateWritePath(path string) error {
	// If no writable paths specified and no restrictions, allow
	if len(e.Context.Filesystem.WritablePaths) == 0 && len(e.Context.Filesystem.ReadOnlyPaths) == 0 {
		return nil
	}

	// Must have writable paths defined for write operations
	if len(e.Context.Filesystem.WritablePaths) == 0 {
		return fmt.Errorf("no writable paths defined in context %q", e.Context.Name)
	}

	for _, allowed := range e.Context.Filesystem.WritablePaths {
		if strings.HasPrefix(path, allowed) {
			return nil
		}
	}

	return fmt.Errorf("path %q not writable in context %q", path, e.Context.Name)
}
