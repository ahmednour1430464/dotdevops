package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	}
}

func runDiagnostics(uri, text string, w *bufio.Writer) {
	path := strings.TrimPrefix(uri, "file://")

	res, _ := devlang.CompileFileAutoDetect(path, []byte(text), "v0.8")

	diagnostics := []Diagnostic{}
	for _, err := range res.Errors {
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
