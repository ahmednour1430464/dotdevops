# .devops Language Versions

This document tracks the evolution of the `.devops` language, marking frozen versions and outlining the design philosophy behind each release.

---

## Version Philosophy

The `.devops` language is built on **compile-time determinism** and **hash stability**. Each version adds orthogonal capabilities that compose cleanly without mutating existing semantics or introducing runtime nondeterminism.

### Design Principles (Invariants)

1. **Compile-time only transformations** — No runtime conditionals, dynamic targets, or execution-time variables
2. **Deterministic expansion** — All macro/loop/step expansion is source-ordered and reproducible
3. **One-way lowering** — High-level constructs (AST) → Low-level plan (JSON), never reversed
4. **Stable hashing** — Identical semantics = identical hash, regardless of source syntax
5. **No semantic leakage** — Version-specific constructs fully expand before reaching the engine

---

## Released Versions

### v0.3 — Expressions & Let Bindings ✅ FROZEN

**Status:** Stable, Production-Ready  
**Release:** February 2026

#### Features
- **Let bindings** with typed expressions (`string`, `number`, `bool`, `list`)
- **Operators:** arithmetic (`+`, `-`, `*`, `/`, `%`), logical (`&&`, `||`, `!`), comparison (`==`, `!=`, `<`, `>`, `<=`, `>=`)
- **String concatenation** (`+` operator)
- **Ternary expressions** (`condition ? true_val : false_val`)
- **List literals** (`[1, 2, 3]`)
- **Type validation** at compile time

#### Philosophy
Established the **expression evaluation layer** as a pure compile-time system. No runtime variable resolution — everything resolves before execution.

#### Test Coverage
- ✅ Valid: comprehensive, concat, logical, ternary
- ✅ Invalid: type_mismatch, unresolved_var
- ✅ Hash stability: expr_version vs literal_version

---

### v0.4 — Reusable Steps (Macro Expansion) ✅ FROZEN

**Status:** Stable, Production-Ready  
**Release:** February 2026

#### Features
- **Step definitions** — Reusable templates for primitives
- **Step invocation** — Reference steps with override inputs
- **Merge precedence** — Invocation inputs override step defaults
- **Memoization** — Expansions cached by step name for efficiency
- **Validation constraints:**
  - Steps cannot use `targets` or `depends_on` (structural fields only)
  - Steps cannot be nested inside primitives
  - Step names must be unique and not collide with primitive names

#### Philosophy
Added **compile-time macros** without introducing runtime behavior. Steps are purely syntactic sugar that expand deterministically into primitives before execution.

#### Test Coverage
- ✅ Valid: basic, comprehensive, multiple_targets, override_inputs, with_lets
- ✅ Invalid: duplicate, nested, collision, undefined, unknown_primitive, with_depends_on, with_targets
- ✅ Hash stability: with_step vs without_step (manual expansion equivalence)

---

### v0.5 — For-Loops & Nested Steps ✅ FROZEN

**Status:** Stable, Production-Ready  
**Release:** February 2026

#### Features
- **For-loops** — Compile-time list iteration with deterministic unrolling
  - `for <item> in <range>`
  - Range must be a let-backed list expression
  - Substitution variable (`item`) scoped to loop body
  - Source-ordered, deterministic expansion
  - No nested loops or steps inside loops
  
- **Nested steps** — Steps can invoke other steps (macro composition)
  - Full cycle detection (DFS + recursion stack)
  - Deep cloning of step bodies
  - Override precedence preserved across nesting levels
  - Deterministic expansion (sorted keys for stability)

#### Philosophy
Completed the **compile-time macro system** with iteration and composition. For-loops eliminate repetition while preserving determinism. Nested steps enable hierarchical abstractions without runtime inheritance.

**Critical Design Choice:**  
Manual expansion and generated expansion produce **identical hashes** — this proves the compiler is semantically correct.

#### Test Coverage
- ✅ Valid: basic, multiple_loops, with_let_range, with_lets, nested_basic, nested_deep, nested_override, comprehensive
- ✅ Invalid: non_list_range, nested_step_cycle_direct, nested_step_cycle_indirect, nested_step_self_reference
- ✅ Hash stability: for_loop_manual vs for_loop_generated, step_expanded vs step_nested

---

### v0.6 — Step Parameters ✅ FROZEN

**Status:** Stable, Production-Ready  
**Release:** February 2026

#### Features
- **Typed parameters** — Steps can declare typed parameters with optional defaults
  - `param name` (required parameter, no default)
  - `param name = <expr>` (optional parameter with default)
  - Parameter types: `string`, `bool`, `list` (using existing expression type system)
  
- **Compile-time substitution** — Parameters resolved during step expansion
  - Parameters shadow lets (formal precedence: paramEnv → letEnv → error)
  - Parameter defaults evaluated once per step definition (compile-time determinism)
  - Node-provided values override step defaults
  
- **Composition** — Parameters work seamlessly with nested steps and for-loops
  - Parameters compose across nested step chains
  - Parameter scope limited to step body (cannot appear in targets, depends_on, node names, or for-loop ranges)

#### Philosophy
Completed the **reusable step abstraction**. Parameters transform steps from fixed macros into configurable, parameterized macros—the missing piece for true code reuse.

**Critical Design Choice:**  
Parameters are **NOT runtime arguments**. They resolve purely at compile time during expansion. After lowering, no param references remain (just like v0.5 has no step references).

#### Test Coverage
- ✅ Valid: param_basic (defaults), param_required (required params provided)
- ✅ Invalid: param_duplicate (duplicate param names), param_missing_required (missing required)
- ✅ Hash stability: Manual expansion == Param-based expansion (semantic equivalence)

---

## Frozen Status

**v0.3, v0.4, v0.5, v0.6 are FROZEN.**

No new syntax or semantics will be added to these versions. They form the stable foundation of the language.

### Why Freeze?

The language now has **complete compile-time macro capabilities:**

1. **Expressions** (v0.3) — Typed, deterministic evaluation
2. **Loops** (v0.5) — Deterministic, source-ordered unrolling
3. **Macro expansion** (v0.4 + v0.5) — Reusable steps with composition
4. **Macro parameterization** (v0.6) — Configurable, typed parameters

Any further additions should **compose on top**, not mutate existing semantics.

---

## Future Versions (Planned)

### v0.7 — Step Libraries (Imports) 🚧 NEXT

**Status:** Planned  
**Target:** Q2 2026

#### Proposed Features
- **Import mechanism** — Load steps from external `.devops` files
- **Namespacing** — Prevent name collisions across libraries
- **Deterministic resolution** — Imports resolve at compile time

#### Why Next?
- v0.6 completes parameterized steps
- Libraries enable code reuse across projects
- Natural evolution: local steps → parameterized steps → importable step libraries

---

### v0.8 — Multi-File Projects 🔮 FUTURE

**Status:** Future Consideration  
**Target:** Q3 2026

---

## What Will NOT Be Added

The following features are **explicitly rejected** to preserve determinism and hash stability:

❌ **Runtime conditionals** — All branching must be compile-time (ternary expressions)  
❌ **Dynamic targets** — Target lists must be statically known  
❌ **Non-literal loops** — Loop ranges must be let-backed lists, not runtime queries  
❌ **Execution-time variables** — All variables resolve before execution  
❌ **Plugin execution hooks** — Primitives remain hermetic and deterministic  

These would break the **hash stability invariant** and introduce nondeterminism.

---

## Testing Philosophy

Each version maintains a comprehensive test matrix:

1. **Structural tests** — Valid and invalid syntax cases
2. **Behavioral tests** — Correct compilation and expansion
3. **Hash stability tests** — Manual vs generated equivalence
4. **Negative tests** — Proper error detection and reporting
5. **Integration tests** — End-to-end validation

**Key Invariant:**  
Manual expansion == Generated expansion (same hash) → Compiler is correct

---

## Language Positioning

The `.devops` language aligns philosophically with:

- **Bazel** — Deterministic build graphs
- **Nix** — Pure, reproducible configuration
- **Terraform** (done right) — Declarative, hash-stable plans

**Not aligned with:**

- **Ansible** — Imperative, runtime-conditional
- **Bash scripts** — Non-deterministic, side-effect driven

This is a **deterministic, hash-stable, macro-based infrastructure language** without runtime concepts.

---

## Version Support Policy

- **Frozen versions** (v0.3, v0.4, v0.5) — Stable, no changes, long-term support
- **Active development** (v0.6) — Current focus, may evolve
- **Future versions** (v0.7+) — Conceptual, subject to change

Breaking changes will only occur across major versions (1.0, 2.0, etc.).

---

## References

- [DESIGN.md](./DESIGN.md) — Core architecture and design philosophy
- [ARCHITECTURAL_AUDIT.md](./.qoder/ARCHITECTURAL_AUDIT.md) — Invariant verification and audits
- [Test suite](./tests/) — Comprehensive validation across all versions

---

**Last Updated:** February 22, 2026  
**Current Stable Version:** v0.6  
**Next Version:** v0.7 (Step Libraries)
