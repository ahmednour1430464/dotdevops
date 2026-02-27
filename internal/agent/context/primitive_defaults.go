package context

import "fmt"

// PrimitiveContextRequirements defines the context requirements for a primitive type.
type PrimitiveContextRequirements struct {
	DefaultContextName string     // Default context name to use
	MinimumTrustLevel  TrustLevel // Minimum required trust level
	RequiresNetwork    bool       // Whether network access is required
	RequiresFilesystem bool       // Whether filesystem access is required
}

// PrimitiveDefaults maps primitive type names to their context requirements.
var PrimitiveDefaults = map[string]PrimitiveContextRequirements{
	"file.sync": {
		DefaultContextName: "safe_user_space",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true,
	},
	"process.exec": {
		DefaultContextName: "safe_user_space",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true, // for cwd
	},
	"_exec": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true,
	},
	"_fs.write": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true,
	},
	"_fs.read": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true,
	},
	"_fs.mkdir": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true,
	},
	"_fs.delete": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelLow,
		RequiresFilesystem: true,
	},
	"_fs.chmod": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelMedium,
		RequiresFilesystem: true,
	},
	"_fs.chown": {
		DefaultContextName: "default",
		MinimumTrustLevel:  TrustLevelHigh,
		RequiresFilesystem: true,
	},
}

// ResolveContext determines which execution context to use for a given primitive.
// It selects the appropriate context based on primitive type and validates
// that the context meets minimum requirements.
func ResolveContext(primitive string, contexts map[string]*ExecutionContext) (*ExecutionContext, error) {
	req, ok := PrimitiveDefaults[primitive]
	if !ok {
		return nil, fmt.Errorf("unknown primitive type: %s", primitive)
	}

	ctx, ok := contexts[req.DefaultContextName]
	if !ok {
		return nil, fmt.Errorf("required context %q not found for primitive %s", 
			req.DefaultContextName, primitive)
	}

	// Validate context meets minimum requirements
	if compareTrustLevel(ctx.TrustLevel, req.MinimumTrustLevel) < 0 {
		return nil, fmt.Errorf("context %q trust level (%s) too low for primitive %s (requires %s)", 
			ctx.Name, ctx.TrustLevel, primitive, req.MinimumTrustLevel)
	}

	return ctx, nil
}

// compareTrustLevel returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareTrustLevel(a, b TrustLevel) int {
	levels := map[TrustLevel]int{
		TrustLevelLow:    1,
		TrustLevelMedium: 2,
		TrustLevelHigh:   3,
	}
	
	aVal, aOk := levels[a]
	bVal, bOk := levels[b]
	
	if !aOk || !bOk {
		return 0 // treat unknown as equal
	}
	
	if aVal < bVal {
		return -1
	} else if aVal > bVal {
		return 1
	}
	return 0
}
