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
		filePath := strings.TrimPrefix(params.TextDocument.URI, "file://")
		if text == "" {
			if data, err := os.ReadFile(filePath); err == nil {
				text = string(data)
			}
		}

		items := GetCompletionsWithFile(text, params.Position.Line, params.Position.Character, filePath)

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
// Supports:
// - Import paths: navigates to the imported file
// - Aliased import references: e.g., lib.web1 → target "web1" in imported file
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

	// First, check if cursor is on an import path string
	for _, decl := range file.Decls {
		if imp, ok := decl.(*devlang.ImportDecl); ok {
			if isPositionInImportPath(imp, targetLine, targetCol, text) {
				return resolveImportPath(imp, path)
			}
		}
	}

	// Second, check if cursor is on an identifier with dot notation (aliased import reference)
	// e.g., lib.web1 or lib.deploy_app
	if loc := resolveAliasedReference(file, text, targetLine, targetCol, path); loc != nil {
		return loc
	}

	return nil
}

// resolveImportPath returns the location of an imported file.
func resolveImportPath(imp *devlang.ImportDecl, fromPath string) *Location {
	resolver := devlang.NewImportResolver(filepath.Dir(fromPath))
	resolvedPath, err := resolver.ResolvePath(imp.Path, fromPath)
	if err != nil {
		return nil
	}

	return &Location{
		URI: "file://" + resolvedPath,
		Range: Range{
			Start: Position{Line: 0, Character: 0},
			End:   Position{Line: 0, Character: 0},
		},
	}
}

// resolveAliasedReference finds the definition for an identifier accessed via aliased import.
// e.g., for "lib.web1" it finds target "web1" in the file imported with alias "lib".
func resolveAliasedReference(file *devlang.File, text string, targetLine, targetCol int, fromPath string) *Location {
	// Find the identifier at the cursor position
	ident := findIdentifierAtPosition(text, targetLine, targetCol)
	if ident == "" {
		return nil
	}

	// Check if it's a dotted identifier (e.g., "lib.web1")
	if !strings.Contains(ident, ".") {
		return nil
	}

	parts := strings.SplitN(ident, ".", 2)
	if len(parts) != 2 {
		return nil
	}
	alias := parts[0]
	memberName := parts[1]

	// Find the import declaration with this alias
	var targetImport *devlang.ImportDecl
	for _, decl := range file.Decls {
		if imp, ok := decl.(*devlang.ImportDecl); ok {
			if imp.Alias == alias {
				targetImport = imp
				break
			}
		}
	}

	if targetImport == nil {
		return nil
	}

	// Resolve the imported file path
	resolver := devlang.NewImportResolver(filepath.Dir(fromPath))
	resolvedPath, err := resolver.ResolvePath(targetImport.Path, fromPath)
	if err != nil {
		return nil
	}

	// Read and parse the imported file
	importedSrc, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil
	}
	importedFile, errs := devlang.ParseFile(resolvedPath, importedSrc)
	if len(errs) > 0 || importedFile == nil {
		return nil
	}

	// Find the definition with the given name
	for _, decl := range importedFile.Decls {
		if pos := getDeclarationPosition(decl, memberName); pos != nil {
			return &Location{
				URI: "file://" + resolvedPath,
				Range: Range{
					Start: Position{Line: pos.Line - 1, Character: pos.Col - 1}, // Convert to 0-based
					End:   Position{Line: pos.Line - 1, Character: pos.Col - 1 + len(memberName)},
				},
			}
		}
	}

	return nil
}

// findIdentifierAtPosition returns the identifier at the given position.
func findIdentifierAtPosition(text string, targetLine, targetCol int) string {
	lines := strings.Split(text, "\n")
	if targetLine <= 0 || targetLine > len(lines) {
		return ""
	}

	line := lines[targetLine-1]
	if len(line) == 0 {
		return ""
	}

	// Convert 1-based column to 0-based index
	// Clamp to valid range
	cursorIdx := targetCol - 1
	if cursorIdx < 0 {
		cursorIdx = 0
	}
	if cursorIdx >= len(line) {
		cursorIdx = len(line) - 1
	}

	// Check if cursor is on an identifier character
	if !isIdentChar(rune(line[cursorIdx])) {
		// Cursor is not on an identifier, try to find nearby
		// Scan left for start of identifier
		found := false
		for i := cursorIdx; i >= 0; i-- {
			if isIdentChar(rune(line[i])) {
				cursorIdx = i
				found = true
				break
			}
		}
		if !found {
			// Scan right for identifier
			for i := cursorIdx; i < len(line); i++ {
				if isIdentChar(rune(line[i])) {
					cursorIdx = i
					found = true
					break
				}
			}
		}
		if !found {
			return ""
		}
	}

	// Scan backwards to find the start of the identifier
	start := cursorIdx
	for start > 0 && isIdentChar(rune(line[start-1])) {
		start--
	}

	// Scan forwards to find the end of the identifier
	end := cursorIdx
	for end < len(line) && isIdentChar(rune(line[end])) {
		end++
	}

	if start >= end {
		return ""
	}

	return line[start:end]
}

// isIdentChar returns true if the character can be part of an identifier.
func isIdentChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.'
}

// getDeclarationPosition returns the position of a named declaration.
func getDeclarationPosition(decl devlang.Decl, name string) *devlang.Position {
	switch d := decl.(type) {
	case *devlang.TargetDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.FleetDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.NodeDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.StepDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.PrimitiveDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.FnDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.LetDecl:
		if d.Name == name {
			return &d.PosInfo
		}
	case *devlang.ModuleDecl:
		if d.Name == name {
			return &d.PosInfo
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
