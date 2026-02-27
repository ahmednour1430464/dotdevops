# DevOpsCTL: Primitive Audit

This document audits existing Go-based primitives and identifies how they will be decomposed into `BUILTINS.md` primitives in the v1.2+ extensibility model.

## 1. `file.sync`
The `file.sync` primitive is currently a complex Go implementation handling diffing, streaming, and snapshotting.

### Decomposition
| Logic Component | Status | New Implementation |
|---|---|---|
| Local directory walking | High-level | Standard Library (expanded at compile-time) |
| SHA256 Diffing | High-level | Standard Library (compile-time comparison) |
| Directory creation | Irreducible | `_fs.mkdir` |
| Atomic File Write | Irreducible | `_fs.write` |
| Mode/Owner enforcement | Irreducible | `_fs.chmod`, `_fs.chown` |
| Extra file deletion | Irreducible | `_fs.delete` |
| Snapshotting | High-level | Standard Library (using `_fs.read` and `_fs.write` to a `.snap` dir) |

## 2. `process.exec`
The `process.exec` primitive maps almost 1:1 to the irreducible set but includes complex retry and failure logic.

### Decomposition
| Logic Component | Status | New Implementation |
|---|---|---|
| Process execution | Irreducible | `_exec` |
| Retry loop | High-level | Controller-side logic (already in `Run`) |
| Failure policy | High-level | Controller-side logic |
| Rollback command | High-level | Standard Library (mapping to another `_exec` call) |

## 3. Migration Roadmap
- **v1.2**: Support the `primitive` keyword.
- **v1.3**: Support `probe` and `rollback` blocks.
- **v1.4**: Port `file.sync` logic from Go to a user-defined primitive in a standard library file (using only built-ins).
