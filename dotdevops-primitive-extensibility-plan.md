# dotdevops — Primitive Extensibility Shipping Plan

**Project:** ahmednour1430464/dotdevops  
**Document Type:** Engineering Roadmap — Phase 2  
**Depends On:** dotdevops-improvement-plan.md (v0.7 through v1.0 must be complete)  
**Scope:** User-defined primitives, the irreducible runtime core, the standard library, and the import/registry ecosystem  
**Versions Covered:** v1.1 through v2.0

---

## Prerequisite: What v1.0 Must Have Delivered

This plan picks up after the previous roadmap ends. Before any work here begins, the following must already be true:

- Plans are self-versioned with a `version` directive (v0.7)
- The agent communicates over mTLS (v0.7)
- Reconcile is grounded in declared state, not execution history (v0.8)
- Node contracts (`idempotent`, `side_effects`, `retry`) exist and are enforced (v0.8)
- Step composability is formally defined: steps are pure compile-time templates (v0.9)
- Secrets are first-class language values with pluggable providers (v0.9)
- The import system works for local files and step definitions (v1.0)
- The LSP exists and provides basic autocompletion and error highlighting (v1.0)

If any of the above are missing, this plan's later phases will be built on an unstable foundation. Do not start v1.1 until v1.0 is shipped and stable.

---

## The Big Picture: What We Are Building

Today, adding a new primitive (`file.sync`, `process.exec`) requires writing Go and shipping a new binary. This plan replaces that with a system where:

1. The Go runtime shrinks to a small set of **irreducible built-in operations** — the absolute minimum that cannot be expressed in terms of anything simpler.
2. A new `primitive` keyword lets anyone define new primitive types as **compositions of those built-ins**, entirely in the `.devops` language.
3. Primitive definitions can be **imported from libraries**, just like step definitions.
4. A curated **standard library** ships with the tool, providing all the primitives that currently live in Go, now written in the language itself.
5. A **registry** lets the community publish and consume primitive libraries.

The end state: the Go runtime only grows when a genuinely new irreducible capability is needed, which should be rare. Everything else lives in the language and grows at community speed.

---

## Version Summary

| Version | Theme | Key Deliverables |
|---------|-------|-----------------|
| v1.1 | Runtime Core | Identify and lock the irreducible built-ins, shrink the Go surface |
| v1.2 | Primitive Keyword | `primitive` block in the language, compile-time expansion, type system for inputs |
| v1.3 | Probe and Rollback | `probe` and `rollback` blocks inside primitives, runtime integration |
| v1.4 | Standard Library | Rewrite existing Go primitives as `.devops` library files using the new system |
| v1.5 | Library Imports | Import primitive definitions from external files, hash pinning |
| v2.0 | Registry | Public registry for primitive libraries, tooling, trust model |

---

## Phase 1 — Lock the Irreducible Core (v1.1)

**Goal:** Decide, precisely and permanently, what the Go runtime is responsible for. This is a design and documentation phase more than an implementation phase. Getting it wrong here forces a breaking change later.

---

### Work Item 1.1.1: Define the Irreducible Built-in Set

The irreducible built-ins are the operations that cannot be expressed as compositions of other operations. They are the atoms of the language. Every other primitive will eventually be built from these.

The proposed set, which must be debated and locked before any implementation begins:

**Filesystem operations:**
- `_fs.write` — write bytes to a path on the target, with mode and owner
- `_fs.read` — read bytes from a path on the target, returns content
- `_fs.delete` — delete a path on the target
- `_fs.exists` — check whether a path exists, returns bool
- `_fs.stat` — return metadata (owner, mode, size, checksum) for a path
- `_fs.mkdir` — create a directory, with mode and owner

**Process operations:**
- `_exec` — run a command with args, env, cwd, and stdin on the target, returns stdout, stderr, exit code

**Network operations:**
- `_net.fetch` — fetch bytes from a URL to a local path on the target

**Signal operations:**
- `_signal` — send a signal to a named process or PID on the target

The leading underscore convention signals to users that these are internal built-ins not meant to be used directly in plans. They are the vocabulary the `primitive` system builds on top of.

The discipline here is important: if someone proposes adding a built-in that could be expressed as a shell command via `_exec`, the answer is no. Every candidate built-in must pass the test: "Can this be implemented using only other built-ins?" If yes, it belongs in the standard library, not in Go.

**Deliverable:** A `BUILTINS.md` document in the repo that names every irreducible built-in, its inputs, its outputs, its probe behavior (if any), and why it cannot be expressed as a composition. This document becomes the contract between the language and the Go runtime. Changing it is a breaking change.

---

### Work Item 1.1.2: Audit and Refactor the Existing Go Primitive Implementations

The current Go implementations of `file.sync` and `process.exec` almost certainly contain logic that goes beyond what the irreducible built-ins cover. Before the new system lands, audit each existing Go primitive and split it into:

- The irreducible operations it depends on (which become built-ins)
- The higher-level logic that composes those operations (which will move to the standard library in v1.4)

This audit produces a clear migration path for v1.4 and also validates that the irreducible built-in set defined in 1.1.1 is complete. If the audit reveals a primitive that genuinely cannot be expressed in terms of the proposed built-ins, add a new built-in and document why.

**Deliverable:** An audit document listing each existing primitive, the built-ins it maps to, and the higher-level logic that will move to the language layer.

---

### Work Item 1.1.3: Version the Built-in Interface

The built-ins are a contract. They will need versioning just like the language itself. Establish from the start that the built-in interface has its own version (`builtins = "v1"` in some internal manifest), separate from the language version. A future change to a built-in's signature is a major version bump. The agent should reject connections from a controller that requests a built-in interface version it does not support, with a clear error message explaining the version mismatch.

---

## Phase 2 — The `primitive` Keyword (v1.2)

**Goal:** Add the `primitive` block to the language. By the end of this version, users can define new primitive types in `.devops` files and use them in plans. Probe and rollback are not yet supported — that comes in v1.3. This version ships the declaration model and compile-time expansion only.

---

### Work Item 1.2.1: The `primitive` Block Syntax

Add `primitive` as a new top-level keyword in the language grammar. A primitive block has three required sections and one optional one:

**Name and version:** The primitive is identified by a dotted name. The name namespace is `category.name`. Built-ins use a leading underscore (`_fs.write`). Standard library primitives use plain names (`file.sync`). User-defined primitives should use a namespaced name to avoid collisions (`mycompany.deploy`).

**The `inputs` section:** Declares the typed parameters the primitive accepts. Each input has a name, a type, and an optional default value. If no default is given, the input is required — a node that uses this primitive without providing that input is a compile error.

The supported input types are: `string`, `int`, `bool`, `list<T>` where T is any scalar type, `map<string, T>` where T is any scalar type, and string enums expressed as `"option1" | "option2" | "option3"`. This type system is intentionally minimal. It covers every real use case and is simple enough to implement and document completely.

**The `body` section:** Declares what the primitive does when applied. The body contains one or more `node` blocks — but these are *inner nodes*, not top-level plan nodes. They use the same syntax as plan nodes but can reference `inputs.*` to substitute parameter values. The body is expanded at compile time: every inner node becomes a real flat plan node, with input references replaced by their concrete values from the calling plan node.

**The `contract` section (optional):** Declares the primitive's behavioral guarantees — the same contract fields introduced in v0.8 (`idempotent`, `side_effects`, `retry`). When a primitive declares a contract, the compiler propagates those declarations to every node that uses the primitive, so operators do not have to repeat them at every call site.

---

### Work Item 1.2.2: The Compiler's Expansion Pass

Primitive expansion is a new compiler pass that runs after parsing and validation but before lowering to the flat plan. The pass works as follows:

Walk every top-level node in the plan. For each node, look up its `type` in the primitive registry. If it is a user-defined primitive, substitute the node's field values into the primitive's input bindings, then recursively expand the primitive's body into concrete nodes. Replace the original node with the expanded nodes in the plan graph.

The expansion is recursive: a primitive's body can use other user-defined primitives, which will themselves be expanded. The recursion must be bounded by a cycle detector. If primitive A's body uses primitive B which uses primitive A, the compiler must detect this and emit an error naming the cycle: "Circular primitive reference: file.sync → company.base_write → file.sync". Expansion depth should also be capped at a reasonable limit (for example, ten levels) to catch accidental deep nesting that is not technically circular but indicates a design problem.

After expansion, the plan contains only built-in operations. The lowering pass that produces the flat JSON plan runs on the already-expanded plan and does not need to know about user-defined primitives at all.

---

### Work Item 1.2.3: Naming and Collision Rules

The primitive registry must have explicit rules about naming:

Built-in names (underscore prefix) are reserved by the runtime and cannot be defined by users. Attempting to define a primitive named `_anything` is a compile error.

Standard library names (`file.*`, `process.*`, `service.*`, etc.) are reserved for the official stdlib. Users can shadow a stdlib primitive with a local definition, but the compiler emits a warning when this happens so the override is visible and intentional.

User namespaces use a dotted prefix: `mycompany.deploy`, `myteam.nginx_vhost`. Any name with at least two dot-separated components where the first component is not a reserved namespace is a valid user primitive name. The compiler does not enforce what the namespace component means — that is a social convention, not a technical one.

---

### Work Item 1.2.4: Source Map Metadata in the Flat Plan

When a user-defined primitive is expanded, the resulting flat plan nodes must carry source map metadata: which primitive definition generated them, with what input values, and at what location in the source plan file. This metadata is written into the flat plan JSON as an optional `_source` field on each node.

The runtime uses source map metadata to produce useful error messages. When a built-in operation fails on the agent, the error message should say not just "node _fs.write failed" but "node deploy-config (file.sync, expanded from plan.devops:14) failed: permission denied writing /etc/nginx/nginx.conf". Without source maps, debugging expanded plans is very difficult.

---

### Work Item 1.2.5: LSP Updates for Primitives

The LSP introduced in v1.0 must be updated to understand the new `primitive` keyword. Specifically:

- Autocompletion should suggest user-defined primitive names in node `type` fields
- Hovering over a node's `type` field should show the primitive's input declarations and contract
- Hovering over an `inputs.*` reference inside a primitive body should resolve to the input's declared type and default
- A missing required input should be flagged as an inline error at the call site

These updates are essential because user-defined primitives are easy to get wrong at the inputs level. The LSP catches these mistakes in the editor rather than at compile time.

---

## Phase 3 — Probe and Rollback in Primitives (v1.3)

**Goal:** Give user-defined primitives the ability to declare their own probe and rollback behavior. This is what makes user-defined primitives as capable as built-in ones — without it, the reconcile and rollback commands cannot work correctly for any primitive that is not in the Go runtime.

---

### Work Item 1.3.1: The `probe` Block

Add an optional `probe` block to the `primitive` syntax. The probe block is a pure expression that describes how to observe the current state of the primitive on a target without changing anything.

The probe expression language is a restricted subset of the full language — no node definitions, no assignments, only function calls that map to built-in read operations (`_fs.exists`, `_fs.stat`, `_fs.read`) and basic expressions (comparisons, conditionals, boolean operators). The probe returns a structured value — a map of named observations — that the runtime compares against the primitive's desired state to determine whether the primitive needs to be applied.

The runtime calls a primitive's probe before deciding whether to run its body. If the probe indicates the desired state is already present, the body is skipped. This is how idempotency works at the primitive level: not as a flag the operator sets, but as a concrete description of what "already done" looks like.

Primitives without a `probe` block are always considered dirty and their body is always run. This is the correct conservative default.

**The desired state contract:** For the probe to be meaningful, the primitive must also declare what "desired state" looks like — the target values that the probe's observations should match. This is expressed as a `desired` block alongside the `probe` block, using `inputs.*` references to derive expected values from the primitive's parameters. The runtime compares `probe` output against `desired` output field by field. Any mismatch means the body needs to run.

---

### Work Item 1.3.2: The `rollback` Block

Add an optional `rollback` block to the `primitive` syntax. The rollback block declares what to do when undoing this primitive's effect. Like the body, it contains inner node definitions that use built-in operations and `inputs.*` references. It also has access to a special `snapshot.*` namespace that references the state captured before the primitive's body ran.

The snapshot is the runtime's responsibility: before running any primitive's body, the runtime calls the probe, captures the probe output, and stores it as the snapshot in the state store. The rollback block can then reference `snapshot.field_name` to access the pre-execution state.

Primitives without a `rollback` block are not rollbackable. When `devopsctl rollback` encounters a node whose primitive has no rollback block and whose `side_effects` is not `none`, it prints an explicit warning and requires operator confirmation before skipping that node. This behavior was established in v0.8 and continues here, now applied uniformly to both built-in and user-defined primitives.

---

### Work Item 1.3.3: Runtime Integration for Probe and Rollback

The runtime's orchestrator must be updated to call primitive probes and rollbacks correctly:

For `apply`: before each node, if its primitive has a probe, run the probe. If the probe indicates desired state is already present, skip the node and record it as `skipped (already satisfied)` in the state store. If the probe indicates divergence or the primitive has no probe, run the body.

For `reconcile`: for each node in the compiled plan, run the probe (if present). Collect all nodes where probe output diverges from desired state. Present the operator with a summary of what is out of sync before making any changes. Then apply only the divergent nodes.

For `observe`: run only probes, never bodies. Print the full diff of desired vs observed state for every node in the plan. No changes are made.

For `rollback`: for each node to be rolled back, check whether its primitive has a rollback block and whether a snapshot exists in the state store. If both are present, execute the rollback block. If either is missing, handle per the v0.8 policy.

---

## Phase 4 — The Standard Library (v1.4)

**Goal:** Rewrite all existing Go primitive implementations as `.devops` library files using the new `primitive` system. After this version ships, the Go runtime contains only the irreducible built-ins. Everything in today's `internal/primitive/` package that goes beyond the built-in surface moves into a standard library written in the language.

---

### Work Item 1.4.1: The Standard Library File Layout

Create a `stdlib/` directory in the repository. It contains `.devops` library files organized by category:

```
stdlib/
  file.devops       — file.sync, file.template, file.chmod, file.chown
  process.devops    — process.exec, process.daemon, process.oneshot
  service.devops    — service.enable, service.start, service.restart, service.stop
  package.devops    — package.install, package.remove, package.update (for apt, yum, apk)
  net.devops        — net.fetch, net.wait_for_port
  user.devops       — user.create, user.modify, user.remove
  dir.devops        — dir.create, dir.sync, dir.clean
```

Each file contains `primitive` blocks only. There are no plan nodes, targets, or fleets in the stdlib. The stdlib is a vocabulary, not a plan.

---

### Work Item 1.4.2: Rewriting `file.sync` as the Reference Example

`file.sync` should be rewritten first, as the reference implementation that all other stdlib primitives follow as a pattern. The rewrite must be verified to produce identical behavior to the Go implementation on a test suite. The test suite should cover: syncing a new file, syncing an updated file, syncing with correct permissions, probing an already-correct file (body must be skipped), and rolling back to the previous file content.

Once `file.sync` is rewritten and verified, the Go implementation in `internal/primitive/` is deleted. Not deprecated — deleted. Keeping the Go implementation alongside the language implementation creates ambiguity about which is canonical.

Follow the same pattern for every other primitive: rewrite, verify, delete the Go implementation. Do not ship v1.4 until all existing Go primitives have been migrated.

---

### Work Item 1.4.3: Implicit Standard Library Import

The standard library should be available in every plan without an explicit import. When the compiler initializes, it loads the stdlib files as the base primitive registry before processing any user imports or plan files. This means existing plans that use `file.sync` and `process.exec` continue to work without any changes — the user experience is identical, but the implementation has moved from Go to the language.

If a user explicitly imports a file that defines a primitive with the same name as a stdlib primitive, the local definition takes precedence and the compiler emits a warning. This is the shadowing mechanism described in v1.2.

---

### Work Item 1.4.4: Stdlib Versioning

The standard library has its own version, separate from the language version and the built-in interface version. Users can pin the stdlib version they depend on with a directive at the top of their plan file:

```
version = "v1.4"
stdlib  = "v1.4.0"
```

If no `stdlib` directive is present, the tool uses the stdlib version bundled with the installed binary. This is the right default for most users. Teams that need reproducible stdlib behavior across binary upgrades should pin the stdlib version explicitly.

The stdlib version follows semver. A patch release fixes bugs without changing primitive behavior. A minor release adds new primitives. A major release is reserved for changes to existing primitive inputs or behavior that could break existing plans.

---

## Phase 5 — Library Imports for Primitives (v1.5)

**Goal:** Allow users to import primitive definitions from external `.devops` files, either local or remote. This is the extension of the v1.0 import system (which handled step definitions) to primitive definitions. After this version ships, the community can publish and consume primitive libraries.

---

### Work Item 1.5.1: Extending Import Syntax for Primitive Libraries

The import system introduced in v1.0 handled step definitions from local files. Extend it to handle primitive definitions from the same sources. The syntax is the same — an `import` statement with an alias — and the compiler handles both step and primitive definitions transparently.

```
import "./my-primitives.devops" as myprims
import "github.com/acme/devops-primitives@v1.2.0" as acme

node "deploy" {
  type    = acme.blue_green_deploy
  targets = [web-fleet]
  artifact = "./build/app"
  slots    = 3
}
```

The alias is required. Aliased imports prevent name collisions between libraries and make the source of a primitive obvious at the call site.

---

### Work Item 1.5.2: Content-Addressed Import Pinning

Remote imports must be pinned to a content hash, not a floating version tag. A version tag like `@v1.2.0` is a hint for humans — the actual pin is the SHA-256 hash of the resolved file content at that version. The tool maintains a lockfile (`.devops.lock`) that records the resolved hash for every remote import:

```
# .devops.lock — generated, do not edit manually
import "github.com/acme/devops-primitives@v1.2.0"
  resolved = "sha256:a3f8c2..."
  fetched  = "2026-03-01"
```

On the first run, the tool resolves the import, downloads the content, computes and stores the hash in the lockfile. On subsequent runs, the tool compares the downloaded content against the stored hash. If they differ, it errors — it does not silently use the new content. The lockfile must be committed to version control. A plan whose lockfile is not committed is not reproducible.

A `devopsctl import update` command updates the lockfile intentionally when the operator wants to upgrade a dependency. This is an explicit, reviewable action — not something that happens automatically.

---

### Work Item 1.5.3: Import Security Model

User-defined primitives that are imported from external sources run on target machines. This is a meaningful security surface. The import system should make the risk visible and give operators tools to manage it.

Every import must be explicitly permitted in a project-level configuration file (`.devopsproject`). An import that appears in a plan file but is not listed in the project configuration is a compile error. This prevents a malicious primitive from being silently added to a plan via a transitive dependency.

The project configuration lists permitted import sources and optionally restricts which primitive names they are allowed to define:

```
# .devopsproject
permitted_imports = [
  { source = "github.com/acme/devops-primitives", allows = ["acme.*"] },
  { source = "./local-primitives.devops",         allows = ["myteam.*"] },
]
```

This is defense in depth — it does not eliminate the risk of a malicious library, but it ensures that imports are a deliberate, visible, reviewable decision rather than a silent transitive effect.

---

### Work Item 1.5.4: `devopsctl import inspect` Command

Add a command that shows the operator exactly what a library import will bring into scope before they commit to using it. For each primitive in the library, the command prints the primitive's name, its inputs and types, its contract declarations, and whether it has probe and rollback blocks. For each built-in operation the primitive uses in its body, those are listed too.

The goal is to make it easy to answer the question "if I import this library, what will it do to my targets?" before running anything. This is the primitive equivalent of reading a dependency's changelog before upgrading.

---

## Phase 6 — The Registry (v2.0)

**Goal:** A public registry where the community can publish, discover, and consume primitive libraries. This is the ecosystem play — the npm or Terraform Registry equivalent for dotdevops primitives.

---

### Work Item 2.0.1: Registry Design Principles

The registry must be designed around a set of principles that differentiate it from the ecosystems that have had security problems:

**No mutable versions.** Once a library version is published at a given hash, neither the hash nor the content can change. Unpublishing is possible (the version is marked unavailable) but the content is never silently replaced. This eliminates the class of attack where a malicious actor publishes a new version under an existing version tag.

**Publisher identity is verified.** Publishing to the registry requires a verified identity (initially GitHub OAuth, potentially other providers). The publisher's identity is displayed prominently on every library page. Users can see who published what and when.

**The official stdlib is the trust anchor.** The standard library published by the dotdevops project is the highest-trust source. Libraries that are widely adopted and have been audited by the project maintainers can receive a "verified" badge, but this is a manual process, not automatic. Most community libraries will have no badge — which is not a warning, just an indication that they have not been formally audited.

**Tooling is the primary security measure.** The registry cannot guarantee the safety of every published primitive. The lockfile, the permitted imports list, and the `import inspect` command are the primary tools users have to manage risk. The registry focuses on making good libraries easy to find and bad ones easy to audit, not on trying to certify safety.

---

### Work Item 2.0.2: Registry CLI Integration

Add registry commands to `devopsctl`:

`devopsctl registry search nginx` — search for libraries by keyword, showing name, publisher, description, download count, and last updated date.

`devopsctl registry info github.com/acme/devops-primitives` — show detailed information about a library: all published versions, all primitives it defines, the publisher's identity, and a link to the source repository.

`devopsctl registry publish ./my-primitives.devops` — publish a library file to the registry. Requires authentication. Validates that the file contains only `primitive` blocks (no plan nodes, no targets). Computes and records the content hash. The published version is permanent.

`devopsctl registry yank v1.2.0` — mark a version as unavailable. Existing users who have pinned this version in their lockfile will see a warning but can still use the pinned content. New users cannot resolve the yanked version. Yanking is for security incidents and serious bugs, not routine updates.

---

### Work Item 2.0.3: The `devops.toml` Project Manifest

By v2.0, a project likely has multiple files, multiple imports, stdlib pinning, and registry dependencies. Consolidate project-level configuration into a single `devops.toml` manifest file:

```toml
[project]
name    = "my-infrastructure"
version = "v2.0"
stdlib  = "v1.6.0"

[[dependency]]
source  = "github.com/acme/devops-primitives"
version = "v1.2.0"
hash    = "sha256:a3f8c2..."
alias   = "acme"
allows  = ["acme.*"]

[[dependency]]
source  = "./local-primitives.devops"
alias   = "myteam"
allows  = ["myteam.*"]
```

The `devops.toml` replaces the `.devopsproject` file introduced in v1.5 and the freestanding lockfile. It is the single source of truth for project configuration. The `devopsctl import update` command updates hashes in this file. It must be committed to version control.

---

## Cross-Cutting Concerns for All Phases

### Error Message Quality

Every new compiler error introduced by this system must follow the same format: state what is wrong, where it is (file and line number), and what to do to fix it. Errors that reference expanded primitives must cite both the expansion site (the plan node) and the definition site (the primitive block), with the source map metadata from v1.2. A user seeing a compile error about a primitive they did not write themselves must be able to understand where the problem is without reading the primitive's source.

### Documentation as a Deliverable

Each version in this plan must ship with complete documentation updates before it is considered done. The documentation deliverables are:

- v1.1: `BUILTINS.md` — the irreducible built-in reference
- v1.2: The `primitive` keyword reference in the language docs, with worked examples
- v1.3: The probe and rollback model explained, with the `observe` command documented
- v1.4: The standard library reference — every stdlib primitive, its inputs, its contract, its probe behavior
- v1.5: The import system extended for primitives, the lockfile format, the security model
- v2.0: The registry user guide — how to find, use, publish, and yank libraries

### Testing Strategy

The primitive expansion system introduces a new class of test: **expansion tests**. An expansion test gives the compiler a plan with a user-defined primitive and asserts that the resulting flat plan matches an expected output exactly. These tests are what guarantee that the compile-to-primitives invariant holds. They should be written for every primitive in the standard library and treated as a regression suite — any change to a stdlib primitive that changes its expansion output is a potentially breaking change.

Probe tests are a second new class: given a target in a known state (set up by test fixtures), assert that the primitive's probe produces a specific output. These are integration tests that require a real or mock target.

The hash stability tests introduced in the original codebase (`test_hash_stability.sh`) must be extended to cover user-defined primitive expansion. The hash of a compiled plan that uses user-defined primitives must be stable across compilations with the same inputs.

---

## What This Plan Does Not Cover

**Cross-target primitives.** The current model assumes a primitive operates on a single target. Some real-world operations span multiple targets (for example, a load balancer reconfiguration that requires coordinating three servers). This is a genuinely hard problem that deserves its own design document. It is not addressed here.

**Primitive testing framework.** Library authors need a way to test their primitives in isolation without a full target. A `devopsctl primitive test` command that runs primitives against a local container or mock target would be extremely valuable, but it is a substantial project on its own. Flag it for a future roadmap.

**Windows agent support.** The irreducible built-ins are defined in terms of POSIX filesystem and process concepts. Supporting Windows targets would require either a Windows-specific built-in set or an abstraction layer. Out of scope for this plan.

---

## Recommended Sequencing

Within each version, the sequencing recommendation is: design document first, then compiler changes, then runtime changes, then LSP updates, then documentation. Never ship a language change without the LSP support — users should never be in a state where the language accepts something that the editor does not understand.

The most critical external dependency for this entire plan is v1.3 (probe and rollback in primitives). It is the version that makes user-defined primitives as capable as built-in ones. If resources are limited, prioritize v1.1 through v1.3 as a coherent first milestone and treat v1.4 through v2.0 as a second milestone. A system with the core primitive model but only the stdlib rewrite partially done is still useful. A system with the stdlib rewrite but no probe support means reconcile does not work for community primitives, which is a serious gap.

---

*This plan is a continuation of dotdevops-improvement-plan.md. Both documents together constitute the complete roadmap from the current state of the project to v2.0.*

*Written February 2026. Revisit after v1.3 ships to assess whether the stdlib rewrite scope in v1.4 should be staged across multiple minor versions.*
