package lsp

// InputSchema describes a primitive input for autocomplete.
type InputSchema struct {
	Type        string // "string", "bool", "list", "int"
	Required    bool   // true if this input must be provided
	Description string // Human-readable description
}

// Schema describes a primitive type for autocomplete.
type Schema struct {
	Name        string                 // e.g., "file.sync"
	Description string                 // Human-readable description
	Inputs      map[string]InputSchema // Input name -> schema
}

// PrimitiveSchemas contains schemas for all built-in primitives.
var PrimitiveSchemas = map[string]Schema{
	"file.sync": {
		Name:        "file.sync",
		Description: "Synchronizes files/directories from source to destination",
		Inputs: map[string]InputSchema{
			"src": {
				Type:        "string",
				Required:    true,
				Description: "Source path (file or directory)",
			},
			"dest": {
				Type:        "string",
				Required:    true,
				Description: "Destination path",
			},
			"mode": {
				Type:        "string",
				Required:    true,
				Description: "File mode (e.g., '0644')",
			},
		},
	},
	"process.exec": {
		Name:        "process.exec",
		Description: "Executes a command on target systems",
		Inputs: map[string]InputSchema{
			"cmd": {
				Type:        "list",
				Required:    true,
				Description: "Command and arguments as a list",
			},
			"cwd": {
				Type:        "string",
				Required:    true,
				Description: "Working directory for command execution",
			},
			"timeout": {
				Type:        "string",
				Required:    false,
				Description: "Execution timeout (e.g., '30s')",
			},
			"env": {
				Type:        "list",
				Required:    false,
				Description: "Environment variables as KEY=VALUE list",
			},
		},
	},
	"_fs.write": {
		Name:        "_fs.write",
		Description: "Built-in: Writes content to a file",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "File path to write",
			},
			"content": {
				Type:        "string",
				Required:    true,
				Description: "Content to write",
			},
			"mode": {
				Type:        "string",
				Required:    false,
				Description: "File mode (e.g., '0644')",
			},
		},
	},
}

// GetSchema returns the schema for a primitive type, or nil if not found.
func GetSchema(primitiveType string) *Schema {
	if s, ok := PrimitiveSchemas[primitiveType]; ok {
		return &s
	}
	return nil
}

// GetInputNames returns all input names for a primitive type.
func GetInputNames(primitiveType string) []string {
	schema := GetSchema(primitiveType)
	if schema == nil {
		return nil
	}
	names := make([]string, 0, len(schema.Inputs))
	for name := range schema.Inputs {
		names = append(names, name)
	}
	return names
}
