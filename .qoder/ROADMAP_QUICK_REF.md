# devopsctl Language Roadmap - Quick Reference

**Status:** Locked and confirmed  
**Philosophy:** Compile-time only, runtime stays flat

---

## 🎯 Current State (v0.4)

✅ **Implemented:**
- Targets and Nodes (v0.1)
- Let bindings (v0.2)
- Expression evaluation: ternary, operators, concat (v0.3)
- Reusable steps (macro expansion) (v0.4)

✅ **Invariants Verified:**
- Lowering is one-way door
- Hashes after full expansion
- Deterministic compilation
- Version-strict validation

---

## 🚀 v0.5: Nested Steps + For-Loops

### For-Loops (Implement FIRST)
**Goal:** Compile-time loop unrolling over literal lists

```devops
let environments = ["dev", "staging", "prod"]

for env in environments {
  node "deploy_${env}" {
    type = file.sync
    targets = [${env}_target]
  }
}
```

**Key Constraints:**
- Loop range MUST be literal (no dynamic iteration)
- Unrolls at compile time
- Loop variable is syntactic substitution only

**Implementation:**
1. Parse `ForDecl` (already exists in AST)
2. Validate range is `ListLiteral`
3. Unroll during lowering
4. Substitute `${var}` in node names and inputs

**Complexity:** Medium | **Risk:** Low

---

### Nested Steps (Implement SECOND)
**Goal:** Allow `step → step`, enforce acyclic graph

```devops
step "base_sync" {
  type = file.sync
  src  = "./build"
}

step "with_backup" {
  type = base_sync
  dest = "/backup"
}

node "deploy" {
  type = with_backup
  targets = [prod]
}
```

**Key Constraints:**
- Steps form DAG
- Cycle detection required (DFS)
- Expand in topological order
- Step → step only (no step → node)

**Implementation:**
1. Build step dependency graph
2. Detect cycles (DFS with visited/stack)
3. Topological sort for expansion order
4. Recursive expansion with memoization
5. Input merging (child overrides parent)

**Complexity:** Medium | **Risk:** Low

---

## 🚀 v0.6: Step Parameters

**Goal:** Typed parameters with defaults

```devops
step "sync_with_options" {
  param backup_enabled = true
  param dest_path              // required (no default)
  
  type = file.sync
  dest = dest_path
}

node "deploy" {
  type = sync_with_options
  dest_path = "/var/www"
  backup_enabled = false
}
```

**Key Constraints:**
- Parameters resolved during expansion
- Do NOT exist after lowering
- Defaults applied at compile time
- Required parameters fail early

**Implementation:**
1. Add `ParamDecl` AST node
2. Extend `StepDecl` with `Params`
3. Validate parameter types
4. Substitute during step expansion

**Complexity:** Medium | **Risk:** Medium

---

## 🚀 v0.7: Step Libraries

**Goal:** Deterministic imports from external files

```devops
// File: stdlib/sync.devops
step "incremental_sync" {
  type = file.sync
  incremental = true
}

// File: myplan.devops
import "./stdlib/sync.devops"

node "deploy" {
  type = incremental_sync
}
```

**Key Constraints:**
- Eager resolution at compile time
- Content-hashed imports
- No lazy loading or network fetches
- Circular import detection

**Implementation:**
1. Add `ImportDecl` AST node
2. Parse imports FIRST
3. Load and parse imported files recursively
4. Detect circular imports
5. Merge step definitions (collision = error)
6. Hash includes all imported content

**Complexity:** High | **Risk:** Medium

---

## 📋 Implementation Order

### Phase 1: v0.5
1. **For-loops** (2-3 days)
   - Simpler, no graph algorithms
   - High value, low risk
   - Test with golden tests

2. **Nested steps** (3-4 days)
   - Requires cycle detection
   - Topological ordering
   - Test with hash stability

### Phase 2: v0.6
3. **Step parameters** (4-5 days)
   - Type system extension
   - Default value handling
   - Parameter validation

### Phase 3: v0.7
4. **Step libraries** (5-7 days)
   - Most complex feature
   - File I/O and import resolution
   - Circular import detection
   - Content-based hashing

---

## 🔒 Non-Negotiables (The Four Invariants)

### 1. Lowering Is a One-Way Door
After lowering: **ONLY** nodes, targets, concrete inputs.  
**No:** step, for, let, param, import.

### 2. Hashes After Full Expansion
Hash = `hash(fully_expanded_plan)`  
**Not:** source, AST, or IR.

### 3. Deterministic Order Everywhere
- Step expansion: topological order
- For-loops: source order
- Imports: sorted paths
- Map iteration: **always sort keys**

### 4. Version-Strict Validation
- Hard version gates
- Explicit rejections
- No feature detection
- Clear error messages

---

## 🚫 What the Language Will NEVER Do

1. Runtime conditionals
2. Runtime loops
3. Runtime variables
4. Dynamic loading during execution
5. Let executor understand high-level constructs
6. Non-deterministic behavior
7. Side effects during compilation (except file I/O for imports)

---

## ✅ What the Compiler Will ALWAYS Do

1. Fully expand all constructs
2. Reject unsupported features by version
3. Produce identical plans for identical inputs
4. Fail fast and loudly
5. Maintain hash stability
6. Validate before lowering
7. Use deterministic iteration order

---

## 🛠️ Development Tools

### Verify Invariants
```bash
./verify_invariants.sh
```

### Run Version Tests
```bash
./test_v0_3.sh  # v0.3 tests
./test_v0_4.sh  # v0.4 tests
# Coming: ./test_v0_5.sh, etc.
```

### Build Compiler
```bash
go build -o ./devopsctl ./cmd/devopsctl
```

### Compile Plan
```bash
./devopsctl plan build --lang=v0.4 myplan.devops
```

---

## 📖 Documentation

- [DESIGN.md](../DESIGN.md) - Canonical design principles
- [INVARIANT_CHECKLIST.md](./INVARIANT_CHECKLIST.md) - Implementation checklist
- [ARCHITECTURAL_AUDIT.md](./ARCHITECTURAL_AUDIT.md) - Current state verification

---

## 🎯 Success Criteria

Each version ships when:
- ✅ All tests pass (valid, invalid, hash stability)
- ✅ Golden tests verify exact output
- ✅ E2E test demonstrates practical usage
- ✅ Invariant verification passes
- ✅ Code review approved
- ✅ Documentation updated

---

**Last Updated:** 2026-02-22  
**Next Milestone:** v0.5 (Nested Steps + For-Loops)
