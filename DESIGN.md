# devopsctl Language Design Principles

## Core Philosophy

**All language features compile to flat, deterministic primitives.**

**The runtime NEVER learns new concepts.**

This document defines non-negotiable architectural invariants that must be preserved across all language versions (v0.1 → v0.∞).

---

## 🔒 The Four Invariants (Non-Negotiable)

### Invariant 1: Lowering Is a One-Way Door

After lowering, ONLY these exist in the final plan:
- ✅ Nodes (with concrete primitive types)
- ✅ Targets (with concrete addresses)
- ✅ Concrete input values

These must NOT exist after lowering:
- ❌ `step` references
- ❌ `for` loops
- ❌ `let` bindings
- ❌ `param` declarations
- ❌ `import` statements
- ❌ Any AST construct beyond primitives

**Mental model:** Lowering is "compiling to assembly". If anything high-level survives lowering, that's a bug.

### Invariant 2: Hashes Are Computed After Full Expansion

The hash input is the **fully expanded plan**, not:
- ❌ Source code
- ❌ AST representation
- ❌ Intermediate representation

This guarantees:
- Step-based vs manual expansion produce identical hashes
- Import safety (imported content affects hash)
- Refactoring without semantic change
- Deterministic builds across environments

**Rule:** Never optimize hash computation by skipping expansion.

### Invariant 3: Deterministic Order Everywhere

Compilation must be deterministic and reproducible:

| Phase | Requirement |
|-------|-------------|
| Step expansion | Topological order (for nested steps) |
| For-loop unrolling | List iteration order (preserve source order) |
| Import resolution | Sorted import paths (deterministic file order) |
| Node emission | Sorted by node ID (if using maps) |

**Rule:** If iteration order depends on a map, sort the keys first.

This protects:
- Hash stability
- CI reproducibility
- Cross-platform compatibility

### Invariant 4: Validation Is Version-Strict

Language validation uses **hard version gates**, not feature detection.

Each `ValidateV0_X` function:
- ✅ Explicitly rejects unsupported constructs
- ✅ Returns clear, actionable errors
- ❌ Never uses "best effort" parsing
- ❌ Never silently ignores unknown features

This prevents:
- Silent semantic drift between versions
- Accidental feature backports
- CI surprises from version mismatches

**Rule:** If a construct isn't explicitly allowed in version X, reject it with a clear error message.

---

## 📋 Feature-Specific Design Locks

### v0.4: Reusable Steps (Implemented)

**Status:** ✅ Stable

Steps are compile-time macros:
- Step definitions are expanded to nodes at compile time
- Step references are resolved during lowering
- No step metadata survives to runtime

**Hash inclusion:** Step definition body is included in plan hash.

---

### v0.5: Nested Steps + For-Loops

#### Nested Steps

**Goal:** Allow `step → step` references with cycle detection.

**Design constraints:**
- Steps form a DAG (Directed Acyclic Graph)
- Nodes are leaves that reference steps
- Step → step resolution happens BEFORE node expansion
- Node references NEVER participate in cycle detection

**Mental model:** Steps are templates; nodes are instantiations.

**Implementation requirements:**
1. Build step dependency graph during `ValidateV0_5`
2. Detect cycles using DFS with visited/stack tracking
3. Expand steps in topological order during `LowerToPlanV0_5`
4. Memoize expanded steps for performance (does not affect semantics)
5. Input merging: child step overrides parent step defaults

**Hash stability:** Include transitive step sources in hash.

#### For-Loops

**Goal:** Compile-time loop unrolling over literal lists.

**Design constraints:**
- Loop range MUST be a literal list (evaluated at compile time)
- No dynamic bounds or runtime variables
- Loop variables are **syntactic substitution only** (not real symbols)

**Mental model:** For-loops are textual expansion, like C preprocessor macros.

**Implementation requirements:**
1. Validate range expression evaluates to `ListLiteral`
2. Unroll loop at compile time during lowering
3. Variable substitution in node names: `${nodename}_${loopvar}`
4. Preserve deterministic expansion order (iterate list in source order)

**Forbidden:**
- Loop variables DO NOT have scope, shadowing rules, or lifetime
- Loop variables DO NOT exist as symbols in the type system

**Hash stability:** Include loop range values in hash.

---

### v0.6: Step Parameters

**Goal:** Allow steps to declare typed parameters with defaults.

**Design constraints:**
- Parameters are resolved during step expansion
- After expansion, parameters DO NOT exist
- Defaults are applied once at compile time
- Required parameters (no default) fail early during validation

**Mental model:** Parameters are compile-time function arguments, not runtime variables.

**Parameter types:**
- `string`
- `bool`
- `list`

**Implementation requirements:**
1. Add `ParamDecl` AST node
2. Extend `StepDecl` with `Params []ParamDecl`
3. Validate parameter name uniqueness within step
4. Validate default value types match parameter types
5. Ensure required parameters are provided at node instantiation
6. Substitute parameter references during step expansion

**Forbidden:**
- Parameters CANNOT reference lets or other parameters
- Parameters are NOT runtime arguments
- Parameters DO NOT survive lowering

**Hash stability:** Include parameter declarations and final values in hash.

---

### v0.7: Step Libraries

**Goal:** Deterministic imports of step definitions from external files.

**Design constraints:**
- All imports resolved eagerly at compile time
- Imported file contents are content-hashed
- No lazy loading, environment-dependent resolution, or network fetches

**Mental model:** Imports are textual inclusion with namespacing, like C `#include`.

**Implementation requirements:**
1. Add `ImportDecl` AST node
2. Parse imports FIRST (before any other declarations)
3. Load and parse imported files recursively
4. Detect circular imports (maintain import graph)
5. Merge step definitions (collision = error)
6. Deterministic import order (sort import paths)

**Path resolution:**
- Relative paths: relative to importing file
- Absolute paths: relative to workspace root

**Forbidden:**
- No dynamic loading during execution
- No conditional imports
- No network-based imports

**Hash stability:** Hash = `hash(main_file_content + sorted_imported_file_contents)`

**Import graph rules:**
- Must be acyclic
- Imports are transitive
- No shadowing (collision = error)

---

## 🚫 Decision Freeze: What the Language Will NEVER Do

The following are **permanent prohibitions**. If a future idea requires violating these, it's a new language, not a new version.

The language will **NEVER**:

1. Have runtime conditionals (no `if` statements during execution)
2. Have runtime loops (no `while` or dynamic `for` during execution)
3. Have runtime variables (no mutable state during execution)
4. Load code dynamically during execution
5. Let the executor understand steps, loops, parameters, or imports
6. Introduce non-deterministic behavior
7. Allow side effects during compilation (beyond file I/O for imports)
8. Support dynamic primitive loading or plugin systems

---

## ✅ Decision Freeze: What the Compiler Will ALWAYS Do

The compiler will **ALWAYS**:

1. Fully expand all high-level constructs to primitives
2. Reject unsupported features by version (with clear errors)
3. Produce identical plans for semantically identical inputs
4. Fail fast and loudly (no silent failures or warnings)
5. Maintain hash stability across refactors
6. Validate before lowering (never produce invalid plans)
7. Use deterministic iteration order (sort when using maps)
8. Include all transitive dependencies in hash computation

---

## 🎯 Why This Matters

This design philosophy enables:

### Executor Independence
- Add new execution backends (cloud, distributed, etc.)
- Change execution strategy without language changes
- Parallelize execution without coordination overhead

### Long-Term Stability
- Plans compiled years ago still execute correctly
- No runtime versioning needed
- No executor-side version detection

### Auditability
- Plans are fully self-contained
- Hash uniquely identifies execution behavior
- No hidden dependencies or dynamic behavior

### Performance
- Execution is flat and predictable
- No interpretation or JIT compilation
- No runtime symbol resolution

### Simplicity
- Runtime is "boring" (in a good way)
- Complexity grows upward (language), not downward (runtime)
- Easy to reason about execution behavior

---

## 📖 How to Use This Document

### For New Features

Before implementing a new language feature:

1. ✅ Check: Does it compile away completely?
2. ✅ Check: Does it preserve deterministic order?
3. ✅ Check: Does it affect hash computation correctly?
4. ✅ Check: Can it be validated at compile time?
5. ❌ If any answer is "no" or "maybe", reconsider the feature.

### For Code Reviews

Reviewers must verify:

1. No high-level constructs survive lowering
2. Hash computation happens after full expansion
3. All iteration uses deterministic order (sorted maps)
4. Version validation is strict (no silent fallbacks)

### For Bug Reports

If a bug violates these invariants, it's a **critical bug** that must be fixed immediately.

---

## 📚 Version History

| Version | Features Added | Design Status |
|---------|----------------|---------------|
| v0.1 | Targets, Nodes, Primitives | ✅ Frozen |
| v0.2 | Let bindings | ✅ Frozen |
| v0.3 | Expression evaluation (ternary, operators) | ✅ Frozen |
| v0.4 | Reusable steps (macro expansion) | ✅ Frozen |
| v0.5 | Nested steps, For-loops | 🔧 In Design |
| v0.6 | Step parameters | 🔧 In Design |
| v0.7 | Step libraries (imports) | 🔧 In Design |

---

## 🔐 Enforcement

This document is **law** for this project.

- All PRs introducing language features must reference this document
- Any deviation from these principles requires explicit architectural review
- If a future idea conflicts with these invariants, it's a new language, not an extension

---

**Last Updated:** 2026-02-22  
**Status:** Canonical Design Document
