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

// GetCompletions returns completion items based on the cursor context.
func GetCompletions(text string, line, character int) []CompletionItem {
	var items []CompletionItem

	// Get the context (what's before the cursor)
	context := analyzeContext(text, line, character)

	switch context.kind {
	case contextTopLevel:
		// Suggest top-level declarations
		items = append(items, keywordCompletions([]string{
			"version", "target", "node", "let", "for", "step", "fleet", "import", "fn", "primitive",
		})...)

	case contextTargetBody:
		// Inside target "name" { ... } - show target-specific fields
		items = append(items, constructFieldCompletions("target")...)

	case contextFleetBody:
		// Inside fleet "name" { ... } - show fleet-specific fields
		items = append(items, constructFieldCompletions("fleet")...)

	case contextNodeType:
		// Suggest primitive types after 'type ='
		items = append(items, primitiveTypeCompletions()...)

	case contextNodeBody:
		// Inside node "name" { ... } - show node-specific fields + primitive inputs
		items = append(items, constructFieldCompletions("node")...)
		if context.primitiveType != "" {
			items = append(items, primitiveInputCompletions(context.primitiveType)...)
		}

	case contextStepBody:
		// Inside step "name" { ... } - show step-specific fields
		items = append(items, constructFieldCompletions("step")...)
		if context.primitiveType != "" {
			items = append(items, primitiveInputCompletions(context.primitiveType)...)
		}

	case contextPrimitiveBody:
		// Inside primitive "name" { ... } - show primitive blocks
		items = append(items, constructFieldCompletions("primitive")...)

	case contextContractBody:
		// Inside contract { ... } - show contract fields
		items = append(items, constructFieldCompletions("contract")...)

	case contextRetryBody:
		// Inside retry { ... } - show retry fields
		items = append(items, constructFieldCompletions("retry")...)

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
	contextTargetBody    // inside target "name" { }
	contextFleetBody     // inside fleet "name" { }
	contextNodeType      // after type =
	contextNodeBody      // inside node "name" { }
	contextStepBody      // inside step "name" { }
	contextPrimitiveBody // inside primitive "name" { }
	contextContractBody  // inside contract { }
	contextRetryBody     // inside retry { }
	contextTargetRef     // inside targets = []
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

	// Look backwards to find which block we're inside
	result := findInnermostBlock(lines, line, beforeCursor)
	if result.kind != contextUnknown {
		return result
	}

	// Check if we're inside 'targets = ['
	if strings.Contains(beforeCursor, "targets =") && strings.Contains(beforeCursor, "[") {
		return completionContext{kind: contextTargetRef}
	}

	// Default to top level
	return completionContext{kind: contextTopLevel}
}

// findInnermostBlock traverses backwards from the current position to find the innermost block context.
func findInnermostBlock(lines []string, currentLineIdx int, beforeCursor string) completionContext {
	braceStack := []blockInfo{}
	primitiveType := ""

	// Process lines from current line going backwards
	for i := currentLineIdx; i >= 0; i-- {
		line := lines[i]
		if i == currentLineIdx {
			line = beforeCursor
		}

		// Process characters from right to left
		for j := len(line) - 1; j >= 0; j-- {
			ch := line[j]
			if ch == '}' {
				// Closing brace - push to stack
				braceStack = append(braceStack, blockInfo{lineNum: i, isOpening: false})
			} else if ch == '{' {
				// Opening brace - check if we have a matching closing brace
				if len(braceStack) > 0 && !braceStack[len(braceStack)-1].isOpening {
					// Pop the matching closing brace
					braceStack = braceStack[:len(braceStack)-1]
				} else {
					// This opening brace is unmatched - we're inside this block
					blockType := identifyBlockType(line[:j])
					return completionContext{kind: blockType, primitiveType: primitiveType}
				}
			}
		}

		// Also scan for 'type = xxx' to capture primitive type
		if idx := strings.Index(line, "type ="); idx >= 0 {
			rest := strings.TrimSpace(line[idx+6:])
			if rest != "" {
				// Extract just the primitive type name
				words := strings.Fields(rest)
				if len(words) > 0 {
					primitiveType = words[0]
				}
			}
		}
	}

	return completionContext{kind: contextUnknown}
}

type blockInfo struct {
	lineNum  int
	isOpening bool
}

// identifyBlockType determines what kind of block precedes an opening brace.
func identifyBlockType(beforeBrace string) contextKind {
	trimmed := strings.TrimSpace(beforeBrace)

	// Check for specific block types by looking at keywords before the brace
	// Order matters - check more specific patterns first

	// Check for retry { }
	if trimmed == "retry" || strings.HasSuffix(trimmed, "retry") {
		return contextRetryBody
	}

	// Check for contract { }
	if trimmed == "contract" || strings.HasSuffix(trimmed, "contract") {
		return contextContractBody
	}

	// Check for target "name" { }
	if strings.HasPrefix(trimmed, "target ") {
		return contextTargetBody
	}

	// Check for fleet "name" { }
	if strings.HasPrefix(trimmed, "fleet ") {
		return contextFleetBody
	}

	// Check for node "name" { }
	if strings.HasPrefix(trimmed, "node ") {
		return contextNodeBody
	}

	// Check for step "name" { }
	if strings.HasPrefix(trimmed, "step ") {
		return contextStepBody
	}

	// Check for primitive "name" { }
	if strings.HasPrefix(trimmed, "primitive ") {
		return contextPrimitiveBody
	}

	return contextUnknown
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

// constructFieldCompletions returns completion items for fields of a language construct.
func constructFieldCompletions(constructType string) []CompletionItem {
	schema := GetConstructSchema(constructType)
	if schema == nil {
		return nil
	}

	items := make([]CompletionItem, 0, len(schema.Fields))
	for name, field := range schema.Fields {
		required := ""
		if field.Required {
			required = " (required)"
		}
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          6, // Variable
			Detail:        field.Type + required,
			Documentation: field.Description,
		})
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
