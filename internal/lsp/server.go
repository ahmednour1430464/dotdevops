package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"devopsctl/internal/devlang"
)

// Position matches LSP's 0-based position.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1 = Error
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// DefinitionParams represents parameters for textDocument/definition request.
type DefinitionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position `json:"position"`
}

// Location represents a location in a document (for definition results).
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// documentCache holds the latest in-memory document text keyed by URI.
// This allows diagnostics and completions to reflect unsaved in-editor changes.
var documentCache = map[string]string{}

func Serve() error {
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	
	for {
		msg, err := readMessage(r)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		var req Request
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}

		handleRequest(req, w)
		w.Flush()
	}
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			contentLength, _ = strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
		}
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	return body, err
}

func handleRequest(req Request, w *bufio.Writer) {
	switch req.Method {
	case "initialize":
		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"capabilities": map[string]any{
					"textDocumentSync": 1, // Full
					"completionProvider": map[string]any{
						"triggerCharacters": []string{".", "=", "["},
						"resolveProvider":   false,
					},
					"definitionProvider": true, // Enable Go to Definition
				},
			},
		}
		send(resp, w)
	case "initialized":
		// Client sends this after receiving initialize response
		// No response needed for notification
	case "textDocument/completion":
		var params CompletionParams
		json.Unmarshal(req.Params, &params)

		// Read from in-memory cache; fall back to disk only if not cached.
		text := documentCache[params.TextDocument.URI]
		if text == "" {
			path := strings.TrimPrefix(params.TextDocument.URI, "file://")
			if data, err := os.ReadFile(path); err == nil {
				text = string(data)
			}
		}

		items := GetCompletions(text, params.Position.Line, params.Position.Character)

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  items,
		}
		send(resp, w)
	case "textDocument/didOpen":
		var params struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		json.Unmarshal(req.Params, &params)
		if params.TextDocument.URI != "" {
			documentCache[params.TextDocument.URI] = params.TextDocument.Text
			runDiagnostics(params.TextDocument.URI, params.TextDocument.Text, w)
		}
	case "textDocument/didChange":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		json.Unmarshal(req.Params, &params)
		if len(params.ContentChanges) > 0 {
			text := params.ContentChanges[len(params.ContentChanges)-1].Text
			documentCache[params.TextDocument.URI] = text
			runDiagnostics(params.TextDocument.URI, text, w)
		}
	case "textDocument/didSave":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		json.Unmarshal(req.Params, &params)
		// Re-run diagnostics on save using cached content.
		if text, ok := documentCache[params.TextDocument.URI]; ok {
			runDiagnostics(params.TextDocument.URI, text, w)
		}
	case "textDocument/didClose":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		json.Unmarshal(req.Params, &params)
		delete(documentCache, params.TextDocument.URI)
	case "textDocument/definition":
		var params DefinitionParams
		json.Unmarshal(req.Params, &params)

		// Read from in-memory cache; fall back to disk only if not cached.
		text := documentCache[params.TextDocument.URI]
		if text == "" {
			path := strings.TrimPrefix(params.TextDocument.URI, "file://")
			if data, err := os.ReadFile(path); err == nil {
				text = string(data)
			}
		}

		location := getDefinition(params.TextDocument.URI, text, params.Position.Line, params.Position.Character)

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  location,
		}
		send(resp, w)
	}
}

func runDiagnostics(uri, text string, w *bufio.Writer) {
	path := strings.TrimPrefix(uri, "file://")

	res, compileErr := devlang.CompileFileAutoDetect(path, []byte(text), "v0.8")

	diagnostics := []Diagnostic{}
	if res == nil && compileErr != nil {
		// Internal compiler failure (e.g. stdlib load error) — surface as a single diagnostic.
		diagnostics = append(diagnostics, Diagnostic{
			Range:    Range{Start: Position{}, End: Position{Character: 5}},
			Severity: 1,
			Source:   "devopsctl",
			Message:  compileErr.Error(),
		})
	}
	var errs []error
	if res != nil {
		errs = res.Errors
	}
	for _, err := range errs {
		line := 0
		col := 0
		msg := err.Error()

		// Use structured position from typed compiler errors (avoids fragile string parsing).
		if se, ok := err.(*devlang.SemanticError); ok {
			line = se.Pos.Line - 1 // LSP is 0-based
			if line < 0 {
				line = 0
			}
			col = se.Pos.Col
			msg = se.Msg
		} else if pe, ok := err.(*devlang.ParseError); ok {
			// ParseError is produced by the parser — use its structured position.
			line = pe.Pos.Line - 1 // LSP is 0-based
			if line < 0 {
				line = 0
			}
			col = pe.Pos.Col
			msg = pe.Msg
		} else if ce, ok := err.(*devlang.CompileError); ok {
			// CompileError fallback — use its structured position.
			line = ce.Line - 1 // LSP is 0-based
			if line < 0 {
				line = 0
			}
			col = ce.Col
			msg = ce.Message
		} else {
			// Last-resort fallback: attempt to parse "file:line:col: message" from error string.
			fmt.Sscanf(msg, "%*s:%d:%d:", &line, &col)
			if line > 0 {
				line-- // LSP is 0-based
			}
		}

		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: line, Character: col},
				End:   Position{Line: line, Character: col + 5},
			},
			Severity: 1,
			Source:   "devopsctl",
			Message:  msg,
		})
	}

	notif := Notification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diagnostics,
		},
	}
	send(notif, w)
}

func send(v any, w *bufio.Writer) {
	msg, _ := json.Marshal(v)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(msg), msg)
}

// getDefinition finds the definition location for the symbol at the given position.
// Currently supports:
// - Import paths: navigates to the imported file
func getDefinition(uri string, text string, line, character int) *Location {
	path := strings.TrimPrefix(uri, "file://")

	// Parse the file to find import declarations
	file, _ := devlang.ParseFile(path, []byte(text))
	if file == nil {
		return nil
	}

	// LSP positions are 0-based, our AST positions are 1-based
	targetLine := line + 1
	targetCol := character + 1

	// Look for an import declaration at the given position
	for _, decl := range file.Decls {
		if imp, ok := decl.(*devlang.ImportDecl); ok {
			// Check if the cursor is within the import path string
			// The import path string appears after 'import' keyword
			// We need to check if the position falls within the string literal
			if isPositionInImportPath(imp, targetLine, targetCol, text) {
				// Resolve the import path
				resolver := devlang.NewImportResolver(filepath.Dir(path))
				resolvedPath, err := resolver.ResolvePath(imp.Path, path)
				if err != nil {
					return nil
				}

				// Return the location of the imported file (start of file)
				return &Location{
					URI: "file://" + resolvedPath,
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: 0, Character: 0},
					},
				}
			}
		}
	}

	return nil
}

// isPositionInImportPath checks if the given position falls within an import path string.
func isPositionInImportPath(imp *devlang.ImportDecl, targetLine, targetCol int, text string) bool {
	// The ImportDecl stores the position of the 'import' keyword
	// We need to find where the path string literal is
	lines := strings.Split(text, "\n")
	if imp.PosInfo.Line <= 0 || imp.PosInfo.Line > len(lines) {
		return false
	}

	// Get the line containing the import declaration
	importLine := lines[imp.PosInfo.Line-1]

	// Find the string literal (the import path) on this line
	// The path is quoted, so we look for the opening and closing quotes
	startQuote := strings.Index(importLine, "\"")
	if startQuote == -1 {
		return false
	}
	endQuote := strings.LastIndex(importLine, "\"")
	if endQuote == -1 || endQuote <= startQuote {
		return false
	}

	// Convert column positions (1-based in the line)
	// startQuote and endQuote are 0-based indices in the line
	// targetCol is 1-based
	if imp.PosInfo.Line == targetLine {
		// +1 because startQuote is 0-based and we want the position after the opening quote
		pathStart := startQuote + 1
		pathEnd := endQuote + 1 // +1 to make it 1-based like targetCol

		// Check if target column falls within the path (inside the quotes)
		if targetCol > pathStart && targetCol < pathEnd {
			return true
		}
	}

	return false
}
