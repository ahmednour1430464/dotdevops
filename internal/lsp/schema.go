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

// ConstructFieldSchema describes a field for a language construct (target, fleet, node, etc.)
type ConstructFieldSchema struct {
	Type        string // "string", "bool", "list", "map", "block", "identifier"
	Required    bool   // true if this field must be provided
	Description string // Human-readable description
}

// ConstructSchema describes a language construct for autocomplete.
type ConstructSchema struct {
	Name        string                          // e.g., "target"
	Description string                          // Human-readable description
	Fields      map[string]ConstructFieldSchema // Field name -> schema
}

// BuiltinFunctionSchema describes a built-in function for autocomplete.
type BuiltinFunctionSchema struct {
	Name        string            // e.g., "_fs.exists"
	Description string            // Human-readable description
	Params      []FunctionParam   // Positional parameters
	Returns     string            // Return type description
	Context     string            // Where it can be used: "probe", "prepare", "any"
}

// FunctionParam describes a function parameter.
type FunctionParam struct {
	Name        string // Parameter name
	Type        string // Parameter type
	Description string // Parameter description
}


// PrimitiveSchemas contains schemas for all built-in primitives.
var PrimitiveSchemas = map[string]Schema{
	// Standard high-level primitives
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
	"template.render": {
		Name:        "template.render",
		Description: "Renders a template file with variables",
		Inputs: map[string]InputSchema{
			"src": {
				Type:        "string",
				Required:    true,
				Description: "Source template path",
			},
			"dest": {
				Type:        "string",
				Required:    true,
				Description: "Destination file path",
			},
			"vars": {
				Type:        "map",
				Required:    false,
				Description: "Template variables",
			},
		},
	},
	"health.check": {
		Name:        "health.check",
		Description: "Performs a health check on a service",
		Inputs: map[string]InputSchema{
			"url": {
				Type:        "string",
				Required:    true,
				Description: "Health check URL",
			},
			"timeout": {
				Type:        "string",
				Required:    false,
				Description: "Check timeout (e.g., '10s')",
			},
		},
	},
	"service.ensure": {
		Name:        "service.ensure",
		Description: "Ensures a service is in the desired state",
		Inputs: map[string]InputSchema{
			"name": {
				Type:        "string",
				Required:    true,
				Description: "Service name",
			},
			"state": {
				Type:        "string",
				Required:    true,
				Description: "Desired state: 'started', 'stopped', 'enabled', 'disabled'",
			},
		},
	},
	"package.install": {
		Name:        "package.install",
		Description: "Installs or removes a system package",
		Inputs: map[string]InputSchema{
			"name": {
				Type:        "string",
				Required:    true,
				Description: "Package name",
			},
			"state": {
				Type:        "string",
				Required:    false,
				Description: "Desired state: 'present' (default) or 'absent'",
			},
		},
	},
	// Irreducible filesystem built-ins (_fs.*)
	"_fs.write": {
		Name:        "_fs.write",
		Description: "Built-in: Writes content to a file on the target",
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
	"_fs.read": {
		Name:        "_fs.read",
		Description: "Built-in: Reads content from a file on the target",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "File path to read",
			},
		},
	},
	"_fs.mkdir": {
		Name:        "_fs.mkdir",
		Description: "Built-in: Creates a directory on the target",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Directory path to create",
			},
			"mode": {
				Type:        "string",
				Required:    false,
				Description: "Directory mode (e.g., '0755')",
			},
		},
	},
	"_fs.delete": {
		Name:        "_fs.delete",
		Description: "Built-in: Deletes a file or directory on the target",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Path to delete",
			},
		},
	},
	"_fs.chmod": {
		Name:        "_fs.chmod",
		Description: "Built-in: Changes file permissions on the target",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "File path",
			},
			"mode": {
				Type:        "string",
				Required:    true,
				Description: "File mode (e.g., '0644')",
			},
		},
	},
	"_fs.chown": {
		Name:        "_fs.chown",
		Description: "Built-in: Changes file owner on the target",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "File path",
			},
			"uid": {
				Type:        "int",
				Required:    false,
				Description: "User ID",
			},
			"gid": {
				Type:        "int",
				Required:    false,
				Description: "Group ID",
			},
		},
	},
	"_fs.exists": {
		Name:        "_fs.exists",
		Description: "Built-in: Checks if a file exists (read-only, for probe blocks)",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Path to check",
			},
		},
	},
	"_fs.stat": {
		Name:        "_fs.stat",
		Description: "Built-in: Returns file metadata (read-only, for probe blocks)",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Path to stat",
			},
		},
	},
	// Irreducible execution built-in
	"_exec": {
		Name:        "_exec",
		Description: "Built-in: Executes a command on the target (irreducible)",
		Inputs: map[string]InputSchema{
			"cmd": {
				Type:        "list",
				Required:    true,
				Description: "Command and arguments as a list",
			},
			"cwd": {
				Type:        "string",
				Required:    false,
				Description: "Working directory",
			},
			"env": {
				Type:        "list",
				Required:    false,
				Description: "Environment variables as KEY=VALUE list",
			},
			"timeout": {
				Type:        "string",
				Required:    false,
				Description: "Execution timeout",
			},
		},
	},
	// Irreducible network built-in
	"_net.fetch": {
		Name:        "_net.fetch",
		Description: "Built-in: Fetches a URL to a local file on the target",
		Inputs: map[string]InputSchema{
			"url": {
				Type:        "string",
				Required:    true,
				Description: "URL to fetch",
			},
			"dest": {
				Type:        "string",
				Required:    true,
				Description: "Destination file path",
			},
			"method": {
				Type:        "string",
				Required:    false,
				Description: "HTTP method (default: GET)",
			},
		},
	},
	// Stdlib primitives
	"file.write": {
		Name:        "file.write",
		Description: "Stdlib: Write content to a file",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Destination file path",
			},
			"content": {
				Type:        "string",
				Required:    true,
				Description: "Content to write",
			},
			"mode": {
				Type:        "string",
				Required:    true,
				Description: "File mode (e.g., '0644')",
			},
		},
	},
	"file.ensure_dir": {
		Name:        "file.ensure_dir",
		Description: "Stdlib: Ensure a directory exists",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Directory path to ensure exists",
			},
			"mode": {
				Type:        "string",
				Required:    false,
				Description: "Directory mode (e.g., '0755')",
			},
		},
	},
	"file.absent": {
		Name:        "file.absent",
		Description: "Stdlib: Ensure a file or directory does not exist",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "Path to ensure does not exist",
			},
		},
	},
	"file.chmod": {
		Name:        "file.chmod",
		Description: "Stdlib: Set file permissions",
		Inputs: map[string]InputSchema{
			"path": {
				Type:        "string",
				Required:    true,
				Description: "File path",
			},
			"mode": {
				Type:        "string",
				Required:    true,
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

// BuiltinFunctionSchemas contains schemas for built-in functions usable in expressions.
var BuiltinFunctionSchemas = map[string]BuiltinFunctionSchema{
	// Filesystem functions (read-only, for probe blocks)
	"_fs.exists": {
		Name:        "_fs.exists",
		Description: "Check if a file or directory exists on the target (read-only)",
		Params: []FunctionParam{
			{Name: "path", Type: "string", Description: "Path to check"},
		},
		Returns: "bool",
		Context: "probe",
	},
	"_fs.stat": {
		Name:        "_fs.stat",
		Description: "Get file metadata on the target (read-only)",
		Params: []FunctionParam{
			{Name: "path", Type: "string", Description: "Path to stat"},
		},
		Returns: "map{exists, is_dir, mode, uid, gid, size, checksum}",
		Context: "probe",
	},
	// Controller functions (for prepare blocks)
	"_ctrl.readdir": {
		Name:        "_ctrl.readdir",
		Description: "Read directory contents on the controller (for prepare blocks)",
		Params: []FunctionParam{
			{Name: "path", Type: "string", Description: "Directory path to read"},
		},
		Returns: "list{relative_path, absolute_path, is_dir, size}",
		Context: "prepare",
	},
	"_ctrl.read": {
		Name:        "_ctrl.read",
		Description: "Read file contents on the controller (for prepare blocks)",
		Params: []FunctionParam{
			{Name: "path", Type: "string", Description: "File path to read"},
		},
		Returns: "string",
		Context: "prepare",
	},
}

// GetBuiltinFunctionSchema returns the schema for a built-in function, or nil if not found.
func GetBuiltinFunctionSchema(name string) *BuiltinFunctionSchema {
	if s, ok := BuiltinFunctionSchemas[name]; ok {
		return &s
	}
	return nil
}

// GetBuiltinFunctionNames returns all built-in function names.
func GetBuiltinFunctionNames() []string {
	names := make([]string, 0, len(BuiltinFunctionSchemas))
	for name := range BuiltinFunctionSchemas {
		names = append(names, name)
	}
	return names
}


// ConstructSchemas contains schemas for all language constructs.
var ConstructSchemas = map[string]ConstructSchema{
	"target": {
		Name:        "target",
		Description: "Declares a target system that can execute nodes",
		Fields: map[string]ConstructFieldSchema{
			"address": {
				Type:        "string",
				Required:    true,
				Description: "Network address of the target (e.g., '1.2.3.4:7700')",
			},
			"labels": {
				Type:        "map",
				Required:    false,
				Description: "Key-value labels for fleet selection (e.g., { role = 'web' })",
			},
		},
	},
	"fleet": {
		Name:        "fleet",
		Description: "Declares a group of targets selected by label matchers",
		Fields: map[string]ConstructFieldSchema{
			"match": {
				Type:        "map",
				Required:    true,
				Description: "Label selector to match targets (e.g., { role = 'web' })",
			},
		},
	},
	"node": {
		Name:        "node",
		Description: "Declares an execution node with a primitive type",
		Fields: map[string]ConstructFieldSchema{
			"type": {
				Type:        "identifier",
				Required:    true,
				Description: "Primitive type (e.g., process.exec, file.sync)",
			},
			"targets": {
				Type:        "list",
				Required:    true,
				Description: "List of target or fleet references",
			},
			"depends_on": {
				Type:        "list",
				Required:    false,
				Description: "List of node names this node depends on",
			},
			"failure_policy": {
				Type:        "string",
				Required:    false,
				Description: "Policy on failure: 'continue' or 'halt'",
			},
			"idempotent": {
				Type:        "bool",
				Required:    false,
				Description: "Mark node as safe for automatic retry",
			},
			"side_effects": {
				Type:        "string",
				Required:    false,
				Description: "Side effect level: 'none', 'local', or 'external'",
			},
			"retry": {
				Type:        "block",
				Required:    false,
				Description: "Retry configuration block { attempts, delay }",
			},
			"rollback_cmd": {
				Type:        "list",
				Required:    false,
				Description: "Command list for rollback operation",
			},
		},
	},
	"step": {
		Name:        "step",
		Description: "Declares a reusable step template with parameters",
		Fields: map[string]ConstructFieldSchema{
			"param": {
				Type:        "identifier",
				Required:    false,
				Description: "Parameter declaration (e.g., param name = 'default')",
			},
			// Node fields are also valid in step body
			"type": {
				Type:        "identifier",
				Required:    true,
				Description: "Primitive type (e.g., process.exec, file.sync)",
			},
			"targets": {
				Type:        "list",
				Required:    true,
				Description: "List of target or fleet references",
			},
			"depends_on": {
				Type:        "list",
				Required:    false,
				Description: "List of node names this node depends on",
			},
			"failure_policy": {
				Type:        "string",
				Required:    false,
				Description: "Policy on failure: 'continue' or 'halt'",
			},
		},
	},
	"primitive": {
		Name:        "primitive",
		Description: "Declares a user-defined primitive with inputs and body",
		Fields: map[string]ConstructFieldSchema{
			"inputs": {
				Type:        "block",
				Required:    false,
				Description: "Block declaring input parameters",
			},
			"body": {
				Type:        "block",
				Required:    true,
				Description: "Body containing node declarations",
			},
			"contract": {
				Type:        "block",
				Required:    false,
				Description: "Contract block declaring behavioral guarantees",
			},
			"probe": {
				Type:        "block",
				Required:    false,
				Description: "Probe block for observing current state",
			},
			"desired": {
				Type:        "block",
				Required:    false,
				Description: "Desired state block for probe comparison",
			},
			"prepare": {
				Type:        "block",
				Required:    false,
				Description: "Prepare block for controller-side computations",
			},
			"foreach": {
				Type:        "block",
				Required:    false,
				Description: "Foreach block for iteration over lists",
			},
		},
	},
	"contract": {
		Name:        "contract",
		Description: "Declares behavioral guarantees for a primitive",
		Fields: map[string]ConstructFieldSchema{
			"idempotent": {
				Type:        "bool",
				Required:    false,
				Description: "Mark primitive as safe for automatic retry",
			},
			"side_effects": {
				Type:        "string",
				Required:    false,
				Description: "Side effect level: 'none', 'local', or 'external'",
			},
			"retry": {
				Type:        "int",
				Required:    false,
				Description: "Number of automatic retry attempts",
			},
		},
	},
	"retry": {
		Name:        "retry",
		Description: "Retry configuration for a node",
		Fields: map[string]ConstructFieldSchema{
			"attempts": {
				Type:        "int",
				Required:    true,
				Description: "Number of retry attempts",
			},
			"delay": {
				Type:        "string",
				Required:    false,
				Description: "Delay between retries (e.g., '10s')",
			},
		},
	},
	"inputs": {
		Name:        "inputs",
		Description: "Declares input parameters for a primitive",
		Fields: map[string]ConstructFieldSchema{
			// Dynamic field names - each is an input name with a type
		},
	},
	"fn": {
		Name:        "fn",
		Description: "Declares a user-defined function (expanded at compile-time)",
		Fields: map[string]ConstructFieldSchema{
			// Function body is an expression, not fields
		},
	},
}

// GetConstructSchema returns the schema for a language construct, or nil if not found.
func GetConstructSchema(constructType string) *ConstructSchema {
	if s, ok := ConstructSchemas[constructType]; ok {
		return &s
	}
	return nil
}

// GetConstructFieldNames returns all field names for a construct type.
func GetConstructFieldNames(constructType string) []string {
	schema := GetConstructSchema(constructType)
	if schema == nil {
		return nil
	}
	names := make([]string, 0, len(schema.Fields))
	for name := range schema.Fields {
		names = append(names, name)
	}
	return names
}
