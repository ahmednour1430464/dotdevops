package lsp

import (
	"strings"
)

// CompletionItem represents an autocomplete suggestion.
type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`          // 1=Text, 2=Method, 3=Function, 6=Variable, 14=Keyword
	Detail        string `json:"detail"`        // Type information
	Documentation string `json:"documentation"` // Description
	InsertText    string `json:"insertText,omitempty"`
}

// CompletionContext represents the completion request context.
type CompletionContext struct {
	TriggerKind int `json:"triggerKind"` // 1=Invoked, 2=TriggerCharacter
}

// CompletionParams represents parameters for completion request.
type CompletionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position           `json:"position"`
	Context  *CompletionContext `json:"context,omitempty"`
}

// Keywords available in v2.0.
var keywords = []string{
	"version", "target", "node", "let", "for", "step", "fleet",
	"module", "primitive", "import", "fn",
	"param", "inputs", "body", "contract", "probe", "desired", "prepare", "foreach",
}

// Built-in primitive types.
var primitiveTypes = []string{
	"file.sync", "process.exec",
}

// Node field keywords.
var nodeFields = []string{
	"type", "targets", "depends_on", "failure_policy",
	"idempotent", "side_effects", "retry", "rollback_cmd",
}

// GetCompletions returns completion items based on the cursor context.
func GetCompletions(text string, line, character int) []CompletionItem {
	var items []CompletionItem

	// Get the context (what's before the cursor)
	context := analyzeContext(text, line, character)

	switch context.kind {
	case contextTopLevel:
		// Suggest top-level declarations
		items = append(items, keywordCompletions([]string{
			"version", "target", "node", "let", "for", "step", "fleet", "import", "fn",
		})...)

	case contextNodeType:
		// Suggest primitive types after 'type ='
		items = append(items, primitiveTypeCompletions()...)

	case contextNodeBody:
		// Suggest node fields and primitive inputs
		items = append(items, nodeFieldCompletions()...)
		if context.primitiveType != "" {
			items = append(items, primitiveInputCompletions(context.primitiveType)...)
		}

	case contextTargetRef:
		// Would need to analyze file for defined targets
		// For now, just show a placeholder
		items = append(items, CompletionItem{
			Label:  "target_name",
			Kind:   6, // Variable
			Detail: "target reference",
		})

	default:
		// Generic completions
		items = append(items, keywordCompletions(keywords)...)
	}

	return items
}

type contextKind int

const (
	contextUnknown contextKind = iota
	contextTopLevel
	contextNodeType
	contextNodeBody
	contextTargetRef
	contextStepBody
)

type completionContext struct {
	kind          contextKind
	primitiveType string
}

func analyzeContext(text string, line, character int) completionContext {
	lines := strings.Split(text, "\n")
	if line >= len(lines) {
		return completionContext{kind: contextTopLevel}
	}

	// Get the current line up to cursor
	currentLine := lines[line]
	if character > len(currentLine) {
		character = len(currentLine)
	}
	beforeCursor := currentLine[:character]

	// Simple heuristics
	trimmed := strings.TrimSpace(beforeCursor)

	// Check if we're after 'type ='
	if strings.HasSuffix(trimmed, "type =") || strings.HasSuffix(trimmed, "type=") {
		return completionContext{kind: contextNodeType}
	}

	// Check if we're in a node body
	// Look backwards for 'node' or 'step' opening
	inNode := false
	primitiveType := ""
	braceDepth := 0
	for i := line; i >= 0; i-- {
		l := lines[i]
		if i == line {
			l = beforeCursor
		}
		for j := len(l) - 1; j >= 0; j-- {
			ch := l[j]
			if ch == '}' {
				braceDepth++
			} else if ch == '{' {
				braceDepth--
				if braceDepth < 0 {
					// Found opening brace
					// Check if it's preceded by 'node' or 'step'
					lineContent := strings.TrimSpace(l[:j])
					if strings.HasPrefix(lineContent, "node ") || strings.HasPrefix(lineContent, "step ") {
						inNode = true
						break
					}
				}
			}
		}
		if inNode {
			break
		}
		// Also scan for 'type = xxx'
		if idx := strings.Index(l, "type ="); idx >= 0 {
			rest := strings.TrimSpace(l[idx+6:])
			if rest != "" {
				primitiveType = strings.Split(rest, " ")[0]
			}
		}
	}

	if inNode {
		return completionContext{kind: contextNodeBody, primitiveType: primitiveType}
	}

	// Check if we're inside 'targets = ['
	if strings.Contains(beforeCursor, "targets =") && strings.Contains(beforeCursor, "[") {
		return completionContext{kind: contextTargetRef}
	}

	// Default to top level
	return completionContext{kind: contextTopLevel}
}

func keywordCompletions(kws []string) []CompletionItem {
	items := make([]CompletionItem, len(kws))
	for i, kw := range kws {
		items[i] = CompletionItem{
			Label:  kw,
			Kind:   14, // Keyword
			Detail: "keyword",
		}
	}
	return items
}

func primitiveTypeCompletions() []CompletionItem {
	items := make([]CompletionItem, 0)
	for name, schema := range PrimitiveSchemas {
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          3, // Function
			Detail:        "primitive type",
			Documentation: schema.Description,
		})
	}
	return items
}

func nodeFieldCompletions() []CompletionItem {
	items := make([]CompletionItem, len(nodeFields))
	for i, field := range nodeFields {
		items[i] = CompletionItem{
			Label:  field,
			Kind:   6, // Variable
			Detail: "node field",
		}
	}
	return items
}

func primitiveInputCompletions(primitiveType string) []CompletionItem {
	schema := GetSchema(primitiveType)
	if schema == nil {
		return nil
	}

	items := make([]CompletionItem, 0, len(schema.Inputs))
	for name, input := range schema.Inputs {
		required := ""
		if input.Required {
			required = " (required)"
		}
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          6, // Variable
			Detail:        input.Type + required,
			Documentation: input.Description,
		})
	}
	return items
}
