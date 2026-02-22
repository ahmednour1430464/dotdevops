# Invariant Enforcement Checklist

Use this checklist when implementing new language features or reviewing PRs.

## 🔍 Pre-Implementation (Design Phase)

- [ ] Feature can be fully expanded at compile time
- [ ] No runtime execution logic required
- [ ] No new executor-side awareness needed
- [ ] Deterministic expansion order is achievable
- [ ] Hash computation strategy is clear
- [ ] Version gate placement is defined
- [ ] Test cases cover all edge cases

## ✅ Implementation Checklist

### AST Changes
- [ ] New AST nodes are documented
- [ ] `Pos()` method implemented for error reporting
- [ ] Parser handles new syntax correctly
- [ ] Parser error messages are clear and actionable

### Validation (`ValidateV0_X`)
- [ ] Unsupported constructs are explicitly rejected
- [ ] Error messages include file, line, column
- [ ] Version-specific validation is isolated to single function
- [ ] No silent fallbacks or "best effort" parsing
- [ ] Symbol tables maintain deterministic order (sorted)
- [ ] Type checking is complete and sound

### Lowering (`LowerToPlanV0_X`)
- [ ] All high-level constructs are eliminated
- [ ] Output contains ONLY: nodes, targets, concrete inputs
- [ ] No `step`, `for`, `let`, `param`, `import` references remain
- [ ] Deterministic expansion order (sort maps before iteration)
- [ ] Input merging is correct (child overrides parent)
- [ ] Error messages are actionable

### Hash Stability
- [ ] Hash includes all transitive dependencies
- [ ] Hash computed AFTER full expansion
- [ ] Semantically equivalent inputs produce identical hashes
- [ ] Expansion order is deterministic
- [ ] Test includes hash stability golden tests

### Testing
- [ ] Valid test cases in `tests/v0_X/valid/`
- [ ] Invalid test cases in `tests/v0_X/invalid/`
- [ ] Hash stability tests in `tests/v0_X/hash_stability/`
- [ ] E2E test covers practical usage
- [ ] Test script `test_v0_X.sh` created and passes
- [ ] Golden tests verify exact JSON output

## 🔒 Invariant Verification

### Invariant 1: Lowering Is a One-Way Door
- [ ] `grep -r "type.*Step" internal/plan/` returns nothing
- [ ] `grep -r "type.*For" internal/plan/` returns nothing
- [ ] `grep -r "type.*Let" internal/plan/` returns nothing
- [ ] `grep -r "type.*Param" internal/plan/` returns nothing
- [ ] `grep -r "type.*Import" internal/plan/` returns nothing
- [ ] Final plan JSON contains only: `targets`, `nodes`, primitive inputs

### Invariant 2: Hashes After Full Expansion
- [ ] Hash computation calls lowering first
- [ ] No shortcuts that skip expansion
- [ ] Test: step-based vs manual expansion produce same hash
- [ ] Test: changing step internals changes hash

### Invariant 3: Deterministic Order
- [ ] All map iterations use sorted keys
- [ ] Expansion order is documented in code comments
- [ ] Test: compilation is reproducible across runs
- [ ] Test: compilation is reproducible across platforms

### Invariant 4: Version-Strict Validation
- [ ] Older versions reject new constructs
- [ ] Error messages mention version requirement
- [ ] No feature detection or silent upgrades
- [ ] Test: v0.X rejects v0.Y features with clear error

## 🧪 Test Coverage Requirements

### Positive Tests (tests/v0_X/valid/)
- [ ] Basic usage of new feature
- [ ] Complex/comprehensive usage
- [ ] Edge cases (empty lists, single element, etc.)
- [ ] Interaction with existing features (lets, steps, etc.)

### Negative Tests (tests/v0_X/invalid/)
- [ ] Syntax errors
- [ ] Type mismatches
- [ ] Undefined references
- [ ] Circular dependencies (if applicable)
- [ ] Invalid nesting (if applicable)

### Hash Stability Tests (tests/v0_X/hash_stability/)
- [ ] Semantically identical inputs produce same hash
- [ ] Functionally equivalent but textually different inputs produce same hash
- [ ] Changing semantics changes hash

### E2E Tests (tests/e2e/)
- [ ] Real-world usage scenario
- [ ] Compile + apply workflow
- [ ] State tracking works correctly

## 📋 Code Review Checklist

### Architecture Review
- [ ] Feature aligns with DESIGN.md principles
- [ ] No violations of decision freeze
- [ ] Compile-time only (no runtime impact)
- [ ] Deterministic behavior guaranteed

### Code Quality
- [ ] Clear error messages with position info
- [ ] Code comments explain non-obvious logic
- [ ] No TODO comments without tracking issues
- [ ] Consistent naming conventions

### Testing
- [ ] Test script passes: `./test_v0_X.sh`
- [ ] No test flakiness
- [ ] Golden tests have clear purpose
- [ ] Test coverage is comprehensive

### Documentation
- [ ] CLI help text updated (if applicable)
- [ ] Language reference updated (if applicable)
- [ ] DESIGN.md updated (if principles changed)
- [ ] Commit message explains "why" not just "what"

## 🚨 Red Flags (Automatic Rejection)

These indicate a violation of core principles:

- [ ] ❌ High-level construct survives lowering
- [ ] ❌ Runtime learns about new language feature
- [ ] ❌ Hash computed before full expansion
- [ ] ❌ Non-deterministic iteration order (unsorted maps)
- [ ] ❌ Version validation uses feature detection
- [ ] ❌ Silent fallbacks or warnings for unsupported features
- [ ] ❌ Dynamic loading or network calls during compilation
- [ ] ❌ Mutable state during execution
- [ ] ❌ Tests are flaky or environment-dependent

## 📝 Sign-Off

Before merging:

- [ ] All checklist items completed
- [ ] No red flags present
- [ ] Tests pass on clean checkout
- [ ] Code reviewed by at least one other developer
- [ ] DESIGN.md updated if principles affected
- [ ] Memory system updated with implementation notes

---

**Purpose:** Ensure all language features maintain architectural integrity.  
**Enforcement:** Required for all PRs touching `internal/devlang/`.
