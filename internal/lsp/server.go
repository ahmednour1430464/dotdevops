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
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func Serve() error {
	r := bufio.NewReader(os.Stdin)
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

		handleRequest(req)
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

func handleRequest(req Request) {
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
		send(resp)
	case "textDocument/completion":
		var params CompletionParams
		json.Unmarshal(req.Params, &params)
		
		// Get document text from cache (simplified - in production, maintain document state)
		// For now, try to read from file
		path := strings.TrimPrefix(params.TextDocument.URI, "file://")
		text := ""
		if data, err := os.ReadFile(path); err == nil {
			text = string(data)
		}
		
		items := GetCompletions(text, params.Position.Line, params.Position.Character)
		
		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  items,
		}
		send(resp)
	case "textDocument/didOpen", "textDocument/didChange", "textDocument/didSave":
		var params struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		json.Unmarshal(req.Params, &params)

		text := params.TextDocument.Text
		if len(params.ContentChanges) > 0 {
			text = params.ContentChanges[0].Text
		}

		if text != "" {
			runDiagnostics(params.TextDocument.URI, text)
		}
	}
}

func runDiagnostics(uri, text string) {
	// Simple diagnostic run using devlang.Compile
	// We extract path from URI (simplified)
	path := strings.TrimPrefix(uri, "file://")
	
	res, _ := devlang.CompileFileAutoDetect(path, []byte(text), "v0.8")
	
	diagnostics := []Diagnostic{}
	for _, err := range res.Errors {
		// Attempt to parse line/col from error string if devlang doesn't expose them cleanly
		// Actually, devlang.CompileResult might have structured errors? Let's check.
		// For now, let's assume res.Errors are strings.
		msg := err.Error()
		line := 0
		col := 0
		fmt.Sscanf(msg, "line %d, col %d", &line, &col)
		if line > 0 {
			line-- // LSP is 0-based
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
	send(notif)
}

func send(v any) {
	msg, _ := json.Marshal(v)
	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(msg), msg)
}
