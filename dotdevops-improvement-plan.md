# dotdevops — Detailed Improvement Plan

**Project:** ahmednour1430464/dotdevops  
**Document Type:** Engineering Roadmap  
**Scope:** Language design, runtime correctness, security, and scalability

---

## Overview

This document identifies the concrete problems in the current dotdevops codebase and language design, explains *why* each is a problem, and prescribes *exactly what to change*. Work is organized into phases tied to version releases, ordered from most critical (correctness and security) to most expansive (language expressiveness and ecosystem). Each phase can ship independently.

---

## Summary of Problems

| # | Problem | Severity | Phase |
|---|---------|----------|-------|
| 1 | Plan files have no self-declared language version | High | v0.7 |
| 2 | Target abstraction has no metadata, labels, or grouping | High | v0.8 |
| 3 | Nodes declare no contracts (idempotency, retry, purity) | High | v0.8 |
| 4 | Agent protocol has no transport security | Critical | v0.7 |
| 5 | Reconcile is grounded in execution history, not declared state | High | v0.8 |
| 6 | Step/macro composability is undefined before import system lands | Medium | v0.9 |
| 7 | No native secret or context injection model | Medium | v0.9 |
| 8 | `.qoder` directory is undocumented | Low | v0.7 |
| 9 | No observe-only mode distinct from reconcile | Medium | v0.8 |
| 10 | Rollback is underspecified and likely unsafe for effectful nodes | High | v0.8 |

---

## Phase 1 — Foundation Fixes (v0.7)

**Goal:** Ship the things that are wrong *right now* and will become more expensive to fix later. Nothing in this phase adds new language features. It is entirely correctness and security hardening.

---

### Problem 1: Plans Have No Self-Declared Version

**What is wrong:**  
The language version is passed as a CLI flag (`--lang v0.6`). This means the plan file itself contains no record of what version it was written for. If someone runs `devopsctl apply plan.devops` without the flag, they get v0.3 by default — even if the plan uses v0.6 features. This silently miscompiles plans rather than failing loudly. The flag also means that stored plans in version control are not self-contained; you must know out-of-band which flag to pass.

**What to change:**  
Add a mandatory `version` directive as the first line of any `.devops` file, for example `version = "v0.6"`. The compiler should read this directive first and set the language version from it. If a `--lang` flag is also passed and it conflicts with the file-declared version, the compiler should error, not silently prefer one. The `--lang` flag should be deprecated but kept temporarily for backwards compatibility, with a warning emitted when it is used. All example plans in the repo should be updated to include the version directive. Documentation should make clear that the version directive is mandatory.

---

### Problem 2: Agent Protocol Has No Transport Security

**What is wrong:**  
The agent listens on a TCP port (default 7700) with no mention of authentication or encryption. Any process that can reach the agent's network address can instruct it to run arbitrary commands. The execution context system controls *what runs with what privileges*, but there is nothing stopping an unauthorized caller from invoking the agent in the first place. In a real environment this is a remote code execution vulnerability by design.

**What to change:**  
Implement mutual TLS (mTLS) for all agent communication. The controller and agent should each hold a certificate, and both sides should verify the other. At startup, the agent should refuse connections from any caller that does not present a trusted certificate. The `devopsctl` CLI should be updated to accept `--cert`, `--key`, and `--ca` flags for the controller side. The agent should accept `--cert`, `--key`, and `--ca` flags for the server side. Provide a `devopsctl pki init` subcommand that generates a self-signed CA, a controller certificate, and an agent certificate for development and homelab use. For production, document how to use an external CA. Until this is shipped, add a prominent security warning to the README and to the agent startup log output that the agent should not be exposed to untrusted networks.

---

### Problem 3: `.qoder` Directory Is Undocumented

**What is wrong:**  
There is a `.qoder` directory in the repository root that is not mentioned anywhere in the README, DESIGN.md, or LANGUAGE_VERSIONS.md. Anyone reading or contributing to the project cannot know what it is, whether it is generated or committed intentionally, or whether it should be in `.gitignore`.

**What to change:**  
Either document the directory's purpose in the README under a "Development Tools" or "Editor Integration" section, or if it is editor-specific tooling state, add it to `.gitignore` and remove it from the repository. If it contains tooling that is genuinely useful to contributors, document it.

---

## Phase 2 — Correctness and Semantics (v0.8)

**Goal:** Fix the problems that make the language semantically unsound. These are design-level issues that touch the language spec, the runtime, and the reconciliation model. This phase will require changes to DESIGN.md and LANGUAGE_VERSIONS.md as well as the implementation.

---

### Problem 4: Target Abstraction Has No Metadata or Grouping

**What is wrong:**  
A target today is defined only by a name and a network address. There is no way to attach metadata to a target (region, role, OS type, environment). There is no way to group targets and refer to the group. The only way to deploy to multiple servers is to list each target explicitly in every node's `targets` field. At ten servers this is tedious. At a hundred it is unworkable. The language has no way to express "deploy this node to all production web servers" without naming each one.

**What to change:**  
Extend the target block to support a `labels` map, for example:

```
target "web1" {
  address = "10.0.0.1:7700"
  labels = {
    role = "web"
    env  = "prod"
    region = "us-east"
  }
}
```

Add a new top-level construct called `fleet` (or `group`) that defines a named collection of targets by label selector:

```
fleet "prod-web" {
  match = { role = "web", env = "prod" }
}
```

Allow nodes to reference fleets in their `targets` field alongside individual targets:

```
node "deploy" {
  type    = file.sync
  targets = [prod-web]
  ...
}
```

The compiler should resolve fleet references during lowering, expanding them into explicit target lists so the flat plan format does not need to change. Fleet resolution should happen after all targets are parsed, so forward references within the same file work correctly.

---

### Problem 5: Nodes Declare No Contracts

**What is wrong:**  
Every node today is treated identically by the runtime: run it, record the result, move on. But a `process.exec` that runs `echo hello` is fundamentally different from one that runs `rm -rf /var/www`. The former is safe to retry automatically; the latter is destructive. The former is idempotent; the latter may not be. The runtime currently has no way to distinguish these cases because nodes carry no declarations about their own behavior. This makes automatic retry, safe parallelism, and rollback planning impossible to implement correctly.

**What to change:**  
Add optional contract fields to node blocks:

```
node "restart-service" {
  type       = process.exec
  targets    = [web1]
  cmd        = ["systemctl", "restart", "myapp"]
  
  idempotent   = true
  retry        = { attempts = 3, delay = "5s" }
  side_effects = "external"  // "none" | "local" | "external"
}
```

`idempotent = true` tells the runtime that running this node twice has the same effect as running it once, so it is safe to retry on transient failure without requiring human confirmation. `side_effects` describes the blast radius of the node: `none` means it can be freely replayed (safe for caching and dry-run), `local` means it changes state on the target only, `external` means it may affect systems outside the target (sending emails, calling APIs, etc.) and should never be automatically retried or replayed.

These fields should be optional — omitting them means the runtime uses conservative defaults (not idempotent, no automatic retry, `side_effects = "local"`). The fields should be validated by the compiler but enforced only at runtime. Document the semantics clearly in LANGUAGE_VERSIONS.md.

---

### Problem 6: Reconcile Is Grounded in Execution History, Not Declared State

**What is wrong:**  
The current `reconcile` command detects drift by comparing the current state of targets against the SQLite execution history — meaning it answers the question "does the world match what we last ran?" rather than "does the world match what we declared?" These are different questions. If the last execution was of an old version of the plan, reconcile will try to restore the old state, not the current declared state. If a node was removed from the plan, reconcile has no way to know that the node's effects should be undone. The execution history is a log, not a truth source.

**What to change:**  
The reconciliation model needs to be re-anchored to the compiled plan, not the state store. Concretely: when `devopsctl reconcile plan.devops` is run, the tool should compile the plan fresh, then for each node in the compiled plan, probe the target to determine current observed state, compare observed state against the node's declared desired state, and apply only the delta. The state store should be used only for recording *what was done* (audit trail), never for computing *what should be done*.

This requires each primitive type to implement a `Probe` interface — a read-only operation that returns the current observed state of that primitive on a target without changing anything. For `file.sync`, the probe reads the current checksum of the destination. For `process.exec`, the probe may not be meaningful (processes are not persistent state), in which case the node is marked as "always re-evaluate on reconcile unless idempotent."

Add a separate `devopsctl observe plan.devops` command that runs only the probe phase and prints a human-readable diff of declared vs observed state, without applying any changes. This is the "what is wrong" command. `reconcile` is the "fix what is wrong" command. They should be distinct.

---

### Problem 7: Rollback Is Underspecified and Likely Unsafe

**What is wrong:**  
`devopsctl rollback --last` exists but the README gives no indication of how it works. For `file.sync`, reverting to a previous file state is conceivable if the previous version was stored. For `process.exec`, there is no meaningful "undo" — you cannot un-run a command. Offering a `rollback` command that silently does nothing (or does the wrong thing) for effectful nodes is dangerous, because operators will rely on it in incident response and be surprised.

**What to change:**  
Define rollback semantics explicitly in DESIGN.md for each primitive type. For `file.sync`: rollback is defined as re-syncing the previous source content to the destination, which requires the state store to snapshot the pre-execution state of the destination before each apply. For `process.exec`: rollback is only supported if the node declares a `rollback_cmd` field, which is a command to run as the inverse operation. If no `rollback_cmd` is declared and the node has `side_effects != "none"`, the rollback command should print an explicit warning that this node cannot be automatically rolled back and require the operator to confirm they understand before proceeding.

The `rollback` command should also print a plan of what it will do — which nodes it can revert, which it cannot, and why — before asking for confirmation. Never silently skip nodes during rollback.

---

## Phase 3 — Language Expressiveness (v0.9)

**Goal:** Add the language features that make dotdevops genuinely more expressive than writing shell scripts. This phase assumes Phase 1 and Phase 2 are complete, because the features here build on correct semantics.

---

### Problem 8: Step Composability Is Undefined Before Imports Land

**What is wrong:**  
The import system (v0.7 in the original roadmap, now being pushed to after the fixes in this plan) will allow users to import step libraries from external files or packages. But the composability semantics of steps/macros are currently undefined: can a step call another step? Can a step have internal conditionals? Can a step return values? If these questions are not answered in the design before import semantics are built, the import system will be built on top of an ambiguous foundation and will need to be redesigned.

**What to change:**  
Before implementing imports, publish a formal definition of what a step is in DESIGN.md. The recommended position is: a step is a pure, parameterized template that expands to a list of nodes at compile time. It has inputs (parameters) and outputs (a list of node definitions). It cannot call other steps recursively, cannot have runtime conditionals (only compile-time ones based on parameter values), and cannot produce side effects during compilation. This keeps the compilation model simple: steps are macros, not functions. Once this is locked in, imports become straightforward — an import brings a set of step definitions into scope, nothing more.

---

### Problem 9: No Native Secret or Context Injection Model

**What is wrong:**  
The execution context system on the agent handles security boundaries, but there is no language-level model for how sensitive values (API keys, passwords, tokens) get from a secret store into a node's parameters. Currently the only option is to hardcode values in the plan file or use environment variables implicitly. Hardcoding secrets in plan files that live in version control is a well-known antipattern. There is no type safety, no auditability, and no rotation path.

**What to change:**  
Add a `secret` reference type to the language. A secret is a typed reference to a value that the controller will resolve at apply time from a configured secret provider, but that is never embedded in the compiled plan or the state store. The syntax should look like:

```
let db_password = secret("DB_PASSWORD")

node "configure-db" {
  type    = process.exec
  targets = [db-server]
  cmd     = ["configure-db", "--password", db_password]
  env     = { DB_PASS = db_password }
}
```

The compiler should accept `secret()` references as a first-class expression type and mark the compiled plan nodes that depend on secrets as requiring resolution before execution. The controller should support pluggable secret providers: initially environment variables and a local file, with a documented interface for adding HashiCorp Vault, AWS Secrets Manager, and others later. Secrets should never appear in dry-run output, audit logs, or the state store — replace them with a placeholder like `[REDACTED]`.

---

## Phase 4 — Ecosystem and Ergonomics (v1.0)

**Goal:** Make dotdevops ready for real-world use by multiple people on a team. This phase is about the experience around the language, not the language itself.

---

### What to build in v1.0

**Import system:** With step composability defined in v0.9, implement `import` statements that bring step definitions from local files or a registry into scope. Start with local file imports only. A registry can come later.

**Language server / editor integration:** Provide a `devopsctl lsp` command that speaks the Language Server Protocol. This gives any LSP-compatible editor (VS Code, Neovim, etc.) autocompletion, inline error highlighting, and hover documentation for `.devops` files. This is also where the `.qoder` directory contents should be properly documented or integrated.

**Plan diffing:** Add `devopsctl diff old.devops new.devops` that shows what changed between two plans at the semantic level (which nodes were added, removed, or changed) rather than at the text level. This is distinct from reconcile — it compares two plan files, not a plan against live infrastructure.

**Structured output:** All commands should support `--output json` for machine-readable output, enabling integration with CI/CD pipelines and monitoring tools.

**License:** The README currently says "Add your license information here." Choose and add a license before v1.0 ships. Without a license, the project is technically all-rights-reserved and contributors cannot legally use it.

---

## Version Summary

| Version | Theme | Key Deliverables |
|---------|-------|-----------------|
| v0.7 | Foundation Fixes | Self-declared plan version, mTLS agent security, `.qoder` documentation |
| v0.8 | Correctness | Target labels and fleets, node contracts, reconcile regrounding, rollback semantics |
| v0.9 | Expressiveness | Step composability spec, secret injection model |
| v1.0 | Ecosystem | Import system, LSP, plan diffing, structured output, license |

---

## Recommended Sequencing Within Each Version

For v0.7: fix the version directive first (it's one day of work and stops the problem from getting worse as more plans are written), then tackle mTLS (which is the most important security issue).

For v0.8: fix reconcile first (it's the conceptually deepest change and will clarify the rollback semantics automatically), then target metadata (it unblocks real-world use), then node contracts (they're additive and don't break existing plans).

For v0.9: write the composability spec document before writing any code. Getting team agreement on paper is faster than refactoring code later.

For v1.0: the LSP is the highest leverage ergonomics investment. Do it before the import system, because the LSP will make the import system much easier to use correctly.

---

*Plan written February 2026. Revisit after v0.8 ships to reprioritize v0.9 and v1.0 based on real usage feedback.*
