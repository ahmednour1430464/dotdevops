package devlang

import "fmt"

// CompileError represents a structured compiler diagnostic with source position.
// It implements the error interface so it is backwards-compatible with all
// existing code that treats compiler errors as plain errors.
type CompileError struct {
	File    string // source file path (may be empty)
	Line    int    // 1-based line number (0 = unknown)
	Col     int    // 1-based column number (0 = unknown)
	Message string // human-readable error message
}

func (e *CompileError) Error() string {
	if e.File != "" && e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Message)
	}
	if e.Line > 0 {
		return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Message)
	}
	return e.Message
}

// newErr is a helper to build a *CompileError from a Position and message.
func newErr(pos Position, file, msg string) *CompileError {
	return &CompileError{File: file, Line: pos.Line, Col: pos.Col, Message: msg}
}

// newErrf is a helper to build a *CompileError with a formatted message.
func newErrf(pos Position, file, format string, args ...any) *CompileError {
	return &CompileError{File: file, Line: pos.Line, Col: pos.Col, Message: fmt.Sprintf(format, args...)}
}
