package devlang

import (
	"embed"
	"fmt"
	"path/filepath"
)

//go:embed stdlib/*.devops
var stdlibFS embed.FS

// StdlibPrimitives holds the parsed stdlib primitive definitions.
var stdlibPrimitives map[string]*PrimitiveDecl

// LoadStdlib parses all stdlib/*.devops files and returns the primitive definitions.
// This is called once at init time to cache stdlib primitives.
func LoadStdlib() (map[string]*PrimitiveDecl, error) {
	if stdlibPrimitives != nil {
		return stdlibPrimitives, nil
	}

	primitives := make(map[string]*PrimitiveDecl)

	entries, err := stdlibFS.ReadDir("stdlib")
	if err != nil {
		// No stdlib directory - this is OK for older versions
		return primitives, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".devops" {
			continue
		}

		content, err := stdlibFS.ReadFile("stdlib/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading stdlib/%s: %w", entry.Name(), err)
		}

		file, errs := ParseFile("stdlib/"+entry.Name(), content)
		if len(errs) > 0 {
			return nil, fmt.Errorf("parsing stdlib/%s: %v", entry.Name(), errs[0])
		}

		// Extract primitives from parsed file
		for _, decl := range file.Decls {
			if prim, ok := decl.(*PrimitiveDecl); ok {
				if _, exists := primitives[prim.Name]; exists {
					return nil, fmt.Errorf("duplicate stdlib primitive %q", prim.Name)
				}
				primitives[prim.Name] = prim
			}
		}
	}

	stdlibPrimitives = primitives
	return primitives, nil
}

// MergeStdlibPrimitives merges stdlib primitives into a primitives map.
// User-defined primitives take precedence over stdlib (allow overriding).
func MergeStdlibPrimitives(userPrimitives map[string]*PrimitiveDecl) (map[string]*PrimitiveDecl, error) {
	stdlib, err := LoadStdlib()
	if err != nil {
		return nil, err
	}

	merged := make(map[string]*PrimitiveDecl)

	// First add stdlib primitives
	for name, prim := range stdlib {
		merged[name] = prim
	}

	// Then add user primitives (overriding stdlib if names conflict)
	for name, prim := range userPrimitives {
		merged[name] = prim
	}

	return merged, nil
}

// init loads stdlib at package initialization time
func init() {
	// Pre-load stdlib (ignore errors during init, will be caught at compile time)
	_, _ = LoadStdlib()
}
