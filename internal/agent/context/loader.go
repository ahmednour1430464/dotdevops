package context

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ContextsFile represents the top-level structure of a contexts configuration file.
type ContextsFile struct {
	Contexts []ExecutionContext `yaml:"contexts"`
}

// LoadContexts reads and validates execution contexts from a YAML file.
// Returns a map of context name to context object.
func LoadContexts(path string) (map[string]*ExecutionContext, error) {
	if path == "" {
		return nil, fmt.Errorf("contexts file path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contexts file %q: %w", path, err)
	}

	var file ContextsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse contexts YAML: %w", err)
	}

	if len(file.Contexts) == 0 {
		return nil, fmt.Errorf("no contexts defined in %q", path)
	}

	contexts := make(map[string]*ExecutionContext, len(file.Contexts))
	for i := range file.Contexts {
		ctx := &file.Contexts[i]
		
		if err := ValidateContext(ctx); err != nil {
			return nil, fmt.Errorf("invalid context %q: %w", ctx.Name, err)
		}

		if _, exists := contexts[ctx.Name]; exists {
			return nil, fmt.Errorf("duplicate context name: %q", ctx.Name)
		}

		contexts[ctx.Name] = ctx
	}

	return contexts, nil
}

// ValidateContext checks that a context is well-formed and internally consistent.
func ValidateContext(ctx *ExecutionContext) error {
	if ctx.Name == "" {
		return fmt.Errorf("context name is required")
	}

	// Validate trust level
	switch ctx.TrustLevel {
	case TrustLevelLow, TrustLevelMedium, TrustLevelHigh:
		// valid
	case "":
		return fmt.Errorf("trust_level is required")
	default:
		return fmt.Errorf("invalid trust_level %q (must be: low, medium, high)", ctx.TrustLevel)
	}

	// Validate identity
	if ctx.Identity.User == "" {
		return fmt.Errorf("identity.user is required")
	}

	// Validate privilege config
	if ctx.Privilege.AllowEscalation {
		if len(ctx.Privilege.SudoCommands) == 0 {
			return fmt.Errorf("privilege.sudo_commands must be specified when allow_escalation is true (use ['*'] for all)")
		}
	}

	// Validate filesystem paths
	for _, path := range ctx.Filesystem.ReadOnlyPaths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("filesystem.readable_paths must contain absolute paths, got: %q", path)
		}
	}
	for _, path := range ctx.Filesystem.WritablePaths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("filesystem.writable_paths must contain absolute paths, got: %q", path)
		}
	}
	for _, path := range ctx.Filesystem.DeniedPaths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("filesystem.denied_paths must contain absolute paths, got: %q", path)
		}
	}

	// Validate audit level
	switch ctx.Audit.Level {
	case AuditLevelMinimal, AuditLevelStandard, AuditLevelFull:
		// valid
	case "":
		return fmt.Errorf("audit.level is required")
	default:
		return fmt.Errorf("invalid audit.level %q (must be: minimal, standard, full)", ctx.Audit.Level)
	}

	// Validate network scope
	if ctx.Network.Scope != "" {
		switch ctx.Network.Scope {
		case "none", "internal", "full":
			// valid
		default:
			return fmt.Errorf("invalid network.scope %q (must be: none, internal, full)", ctx.Network.Scope)
		}
	}

	return nil
}
