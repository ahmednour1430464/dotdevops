package lsp

import (
	"os"
	"path/filepath"
	"strings"

	"devopsctl/internal/devlang"
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
	return GetCompletionsWithFile(text, line, character, "")
}

// GetCompletionsWithFile returns completion items based on the cursor context, with file path for import resolution.
func GetCompletionsWithFile(text string, line, character int, filePath string) []CompletionItem {
	var items []CompletionItem

	// Get the context (what's before the cursor)
	ctx := analyzeContext(text, line, character)
	ctx.filePath = filePath
	ctx.fileContent = text

	switch ctx.kind {
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
		if ctx.primitiveType != "" {
			items = append(items, primitiveInputCompletions(ctx.primitiveType)...)
		}

	case contextStepBody:
		// Inside step "name" { ... } - show step-specific fields
		items = append(items, constructFieldCompletions("step")...)
		if ctx.primitiveType != "" {
			items = append(items, primitiveInputCompletions(ctx.primitiveType)...)
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

	case contextProbeBody:
		// Inside probe { ... } - show built-in functions for probes
		items = append(items, builtinFunctionCompletions("probe")...)

	case contextDesiredBody:
		// Inside desired { ... } - show common desired state fields
		items = append(items, desiredFieldCompletions()...)

	case contextPrepareBody:
		// Inside prepare { ... } - show controller built-in functions
		items = append(items, builtinFunctionCompletions("prepare")...)

	case contextAliasDot:
		// After alias. (e.g., lib.) - show declarations from imported file
		items = append(items, aliasMemberCompletions(ctx)...)

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
	contextProbeBody     // inside probe { }
	contextDesiredBody   // inside desired { }
	contextPrepareBody   // inside prepare { }
	contextAliasDot      // after alias. (e.g., lib.)
)

type completionContext struct {
	kind          contextKind
	primitiveType string
	aliasName     string // for contextAliasDot: the alias being accessed
	filePath      string // path to the current file (for resolving imports)
	fileContent   string // content of the current file
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

	// Check if we're after 'type =' (this should work in any context)
	if strings.HasSuffix(trimmed, "type =") || strings.HasSuffix(trimmed, "type=") {
		return completionContext{kind: contextNodeType}
	}

	// Check if we're after an alias followed by a dot (e.g., "lib." or "[lib.")
	// This should work inside targets = [lib.] or anywhere else
	if alias := detectAliasDotAccess(beforeCursor); alias != "" {
		return completionContext{kind: contextAliasDot, aliasName: alias}
	}

	// Look backwards to find which block we're inside
	result := findInnermostBlock(lines, line, beforeCursor)
	if result.kind != contextUnknown {
		// Check for 'type =' within the block context
		if strings.Contains(trimmed, "type =") || strings.Contains(trimmed, "type=") {
			// Check if we're AFTER 'type =' (cursor is after the = sign)
			if idx := strings.LastIndex(trimmed, "type ="); idx >= 0 {
				afterType := trimmed[idx+6:]
				if afterType == "" || afterType == " " {
					return completionContext{kind: contextNodeType}
				}
			} else if idx := strings.LastIndex(trimmed, "type="); idx >= 0 {
				afterType := trimmed[idx+5:]
				if afterType == "" || afterType == " " {
					return completionContext{kind: contextNodeType}
				}
			}
		}
		return result
	}

	// Check if we're inside 'targets = ['
	if strings.Contains(beforeCursor, "targets =") && strings.Contains(beforeCursor, "[") {
		return completionContext{kind: contextTargetRef}
	}

	// Default to top level
	return completionContext{kind: contextTopLevel}
}

// detectAliasDotAccess checks if the cursor is after an alias followed by a dot.
// Returns the alias name if detected, empty string otherwise.
// Works with patterns like: "lib.", "[lib.", "targets = [lib.", ", lib."
func detectAliasDotAccess(beforeCursor string) string {
	trimmed := strings.TrimSpace(beforeCursor)

	// Look for pattern: alias. at the end
	// We need to find an identifier followed by a dot at the very end
	if !strings.HasSuffix(trimmed, ".") {
		return ""
	}

	// Remove the trailing dot
	beforeDot := trimmed[:len(trimmed)-1]

	// Handle cases like "[lib" or ", lib" - extract just the identifier
	// Find the start of the identifier (after any non-identifier chars)
	start := len(beforeDot)
	for i := len(beforeDot) - 1; i >= 0; i-- {
		ch := beforeDot[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			start = i
		} else {
			break
		}
	}

	if start >= len(beforeDot) {
		return ""
	}

	alias := beforeDot[start:]

	// Simple check: the alias should be alphanumeric with underscores
	if alias == "" {
		return ""
	}

	return alias
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

	// Check for probe { }
	if trimmed == "probe" || strings.HasSuffix(trimmed, "probe") {
		return contextProbeBody
	}

	// Check for desired { }
	if trimmed == "desired" || strings.HasSuffix(trimmed, "desired") {
		return contextDesiredBody
	}

	// Check for prepare { }
	if trimmed == "prepare" || strings.HasSuffix(trimmed, "prepare") {
		return contextPrepareBody
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

// builtinFunctionCompletions returns completions for built-in functions usable in the given context.
func builtinFunctionCompletions(context string) []CompletionItem {
	items := make([]CompletionItem, 0)
	for name, schema := range BuiltinFunctionSchemas {
		// Filter by context if specified
		if context != "" && schema.Context != context && schema.Context != "any" {
			continue
		}
		// Build parameter hint for insert text
		paramHint := ""
		insertText := name
		if len(schema.Params) > 0 {
			insertText = name + "(${1})"
			paramHint = "("
			for i, p := range schema.Params {
				if i > 0 {
					paramHint += ", "
				}
				paramHint += p.Type
			}
			paramHint += ")"
		} else {
			paramHint = "()"
		}

		items = append(items, CompletionItem{
			Label:         name,
			Kind:          3, // Function
			Detail:        paramHint + " -> " + schema.Returns,
			Documentation: schema.Description,
			InsertText:    insertText,
		})
	}
	return items
}

// desiredFieldCompletions returns completions for common desired state fields.
func desiredFieldCompletions() []CompletionItem {
	return []CompletionItem{
		{
			Label:         "exists",
			Kind:          6, // Variable
			Detail:        "bool",
			Documentation: "Whether the file/directory should exist",
		},
		{
			Label:         "is_dir",
			Kind:          6, // Variable
			Detail:        "bool",
			Documentation: "Whether the path should be a directory",
		},
		{
			Label:         "mode",
			Kind:          6, // Variable
			Detail:        "string",
			Documentation: "Expected file mode (e.g., '0644')",
		},
		{
			Label:         "checksum",
			Kind:          6, // Variable
			Detail:        "string",
			Documentation: "Expected SHA256 checksum",
		},
		{
			Label:         "content",
			Kind:          6, // Variable
			Detail:        "string",
			Documentation: "Expected file content",
		},
	}
}

// aliasMemberCompletions returns completions for members of an aliased import.
func aliasMemberCompletions(ctx completionContext) []CompletionItem {
	if ctx.filePath == "" || ctx.aliasName == "" {
		return nil
	}

	// Parse the current file to find the import declaration with this alias
	file, _ := devlang.ParseFile(ctx.filePath, []byte(ctx.fileContent))
	if file == nil {
		return nil
	}

	// Find the import declaration with the matching alias
	var targetImport *devlang.ImportDecl
	for _, decl := range file.Decls {
		if imp, ok := decl.(*devlang.ImportDecl); ok {
			if imp.Alias == ctx.aliasName {
				targetImport = imp
				break
			}
		}
	}

	if targetImport == nil {
		return nil
	}

	// Resolve the import path
	resolver := devlang.NewImportResolver(filepath.Dir(ctx.filePath))
	resolvedPath, err := resolver.ResolvePath(targetImport.Path, ctx.filePath)
	if err != nil {
		return nil
	}

	// Read and parse the imported file
	importedSrc, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil
	}
	importedFile, _ := devlang.ParseFile(resolvedPath, importedSrc)
	if importedFile == nil {
		return nil
	}

	// Collect all declarations from the imported file
	items := make([]CompletionItem, 0)
	for _, decl := range importedFile.Decls {
		item := declarationToCompletionItem(decl)
		if item.Label != "" {
			items = append(items, item)
		}
	}

	return items
}

// declarationToCompletionItem converts a declaration to a completion item.
func declarationToCompletionItem(decl devlang.Decl) CompletionItem {
	switch d := decl.(type) {
	case *devlang.TargetDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        "target",
			Documentation: "Target: " + d.Name,
		}
	case *devlang.FleetDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        "fleet",
			Documentation: "Fleet: " + d.Name,
		}
	case *devlang.NodeDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        "node",
			Documentation: "Node: " + d.Name,
		}
	case *devlang.StepDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        "step",
			Documentation: "Step: " + d.Name,
		}
	case *devlang.PrimitiveDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          3, // Function
			Detail:        "primitive",
			Documentation: "Primitive: " + d.Name,
		}
	case *devlang.FnDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          3, // Function
			Detail:        "fn",
			Documentation: "Function: " + d.Name,
		}
	case *devlang.LetDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        "let",
			Documentation: "Variable: " + d.Name,
		}
	case *devlang.ModuleDecl:
		return CompletionItem{
			Label:         d.Name,
			Kind:          6, // Variable
			Detail:        "module",
			Documentation: "Module: " + d.Name,
		}
	default:
		return CompletionItem{}
	}
}
