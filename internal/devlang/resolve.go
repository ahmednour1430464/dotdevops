package devlang

import (
	"fmt"
	"os"
	"path/filepath"
)

// ImportResolver handles resolving and loading imported files.
type ImportResolver struct {
	// BaseDir is the directory to resolve relative imports from
	BaseDir string
	// Loaded tracks already-loaded files to prevent duplicates
	Loaded map[string]*File
	// Resolved tracks files whose imports have been fully resolved
	Resolved map[string][]Decl
	// InProgress tracks files currently being resolved (for cycle detection)
	InProgress map[string]bool
}

// NewImportResolver creates a new resolver rooted at the given directory.
func NewImportResolver(baseDir string) *ImportResolver {
	return &ImportResolver{
		BaseDir:    baseDir,
		Loaded:     make(map[string]*File),
		Resolved:   make(map[string][]Decl),
		InProgress: make(map[string]bool),
	}
}

// ResolvePath resolves an import path to an absolute file path.
// - "./foo.devops" → relative to current file
// - "/abs/path.devops" → absolute path
func (r *ImportResolver) ResolvePath(importPath string, fromFile string) (string, error) {
	if filepath.IsAbs(importPath) {
		return importPath, nil
	}

	// Relative path: resolve from the directory containing the importing file
	baseDir := filepath.Dir(fromFile)
	if baseDir == "" {
		baseDir = r.BaseDir
	}

	resolved := filepath.Join(baseDir, importPath)
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolving import path %q: %w", importPath, err)
	}

	return absPath, nil
}

// LoadFile loads and parses a .devops file.
// Returns the parsed File and any errors encountered.
func (r *ImportResolver) LoadFile(path string) (*File, []error) {
	// Normalize path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, []error{fmt.Errorf("resolving path %q: %w", path, err)}
	}

	// Check if already loaded
	if file, ok := r.Loaded[absPath]; ok {
		return file, nil
	}

	// Read file
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, []error{fmt.Errorf("reading %s: %w", absPath, err)}
	}

	// Parse file
	file, errs := ParseFile(absPath, src)
	if len(errs) > 0 {
		return nil, errs
	}

	// Cache the loaded file
	r.Loaded[absPath] = file

	return file, nil
}

// ResolveImports processes all import declarations in a file, recursively loading
// imported files and merging their declarations into the scope.
// Returns the merged declarations and any errors.
func (r *ImportResolver) ResolveImports(file *File) ([]Decl, []error) {
	absPath, _ := filepath.Abs(file.Path)

	// Check if already resolved
	if decls, ok := r.Resolved[absPath]; ok {
		return decls, nil
	}

	// Check for circular import
	if r.InProgress[absPath] {
		return nil, []error{fmt.Errorf("circular import detected: %s", absPath)}
	}

	// Mark as in progress
	r.InProgress[absPath] = true
	defer func() {
		delete(r.InProgress, absPath)
	}()

	var allDecls []Decl
	var allErrors []error

	for _, decl := range file.Decls {
		if imp, ok := decl.(*ImportDecl); ok {
			// Resolve the import path
			resolved, err := r.ResolvePath(imp.Path, file.Path)
			if err != nil {
				allErrors = append(allErrors, &ParseError{
					Path: file.Path,
					Pos:  imp.PosInfo,
					Msg:  err.Error(),
				})
				continue
			}

			// Check if the resolved import creates a cycle BEFORE loading
			if r.InProgress[resolved] {
				return nil, []error{fmt.Errorf("circular import detected: %s imports %s", file.Path, resolved)}
			}

			// Load the imported file
			importedFile, errs := r.LoadFile(resolved)
			if len(errs) > 0 {
				allErrors = append(allErrors, errs...)
				continue
			}

			// Recursively resolve imports in the imported file
			importedDecls, errs := r.ResolveImports(importedFile)
			if len(errs) > 0 {
				allErrors = append(allErrors, errs...)
				continue
			}

			// Add imported declarations (but not imports or version)
			for _, d := range importedDecls {
				switch d.(type) {
				case *ImportDecl, *VersionDecl:
					// Skip import and version declarations from imported files
					continue
				default:
					allDecls = append(allDecls, d)
				}
			}
		} else {
			// Keep non-import declarations
			allDecls = append(allDecls, decl)
		}
	}

	// Cache the resolved declarations if no errors
	if len(allErrors) == 0 {
		r.Resolved[absPath] = allDecls
	}

	return allDecls, allErrors
}

// ResolveFile is the main entry point for resolving a file with all its imports.
// Returns a new File with all imports resolved and merged.
func (r *ImportResolver) ResolveFile(path string) (*File, []error) {
	// Load the main file
	file, errs := r.LoadFile(path)
	if len(errs) > 0 {
		return nil, errs
	}

	// Resolve all imports
	decls, resolveErrs := r.ResolveImports(file)
	if len(resolveErrs) > 0 {
		return nil, resolveErrs
	}

	// Create a new file with resolved declarations
	return &File{
		Path:  file.Path,
		Decls: decls,
	}, nil
}
