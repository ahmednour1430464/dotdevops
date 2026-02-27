# DevOpsCTL: Irreducible Built-ins (v1)

This document defines the set of **irreducible built-ins** supported by the `devopsctl` agent. These are the lowest-level operations that cannot be further decomposed into other DevOpsCTL primitives.

## 1. Interface Versioning
The built-in set follows a strict versioning scheme.
- **Current Version**: `v1`
- **Compatibility**: Agents MUST reject plans requesting a higher built-in version than they support.

## 2. File System Primitives (`_fs.*`)

### `_fs.write`
- **Inputs**: `path` (string), `content` (string|bytes), `mode` (optional octal)
- **Description**: Writes content to a file. Overwrites if exists.
- **Probe**: Returns the SHA256 hash of the existing file.

### `_fs.read`
- **Inputs**: `path` (string)
- **Description**: Reads content from a file.
- **Restrictions**: Restricted to small files (e.g. < 1MB) for safety.

### `_fs.delete`
- **Inputs**: `path` (string)
- **Description**: Deletes a file or empty directory.
- **Probe**: Returns `exists` (bool).

### `_fs.mkdir`
- **Inputs**: `path` (string), `mode` (optional octal)
- **Description**: Creates a directory and parents.
- **Probe**: Returns `exists` (bool) and `is_dir` (bool).

### `_fs.chmod` / `_fs.chown`
- **Inputs**: `path` (string), `mode`/`uid`/`gid`
- **Description**: Changes file metadata.
- **Probe**: Returns current metadata values.

### `_fs.exists`
- **Inputs**: `path` (string)
- **Description**: Checks whether a file or directory exists at the given path.
- **Returns**: `exists` (bool)
- **Side Effects**: None (read-only operation)

### `_fs.stat`
- **Inputs**: `path` (string)
- **Description**: Returns file metadata for the given path.
- **Returns**: `exists` (bool), `is_dir` (bool), `mode` (octal string), `uid` (int), `gid` (int), `size` (int), `checksum` (SHA256 hex string, empty for directories)
- **Side Effects**: None (read-only operation)

## 3. Execution Primitives (`_exec.*`)

### `_exec`
- **Inputs**: `cmd` (string array), `cwd` (string), `env` (map), `timeout` (duration)
- **Description**: Executes a local process.
- **Probe**: None (Execution is an effect). Probing must be done via `_fs` or other side-effect-free built-ins.

## 4. Networking Primitives (`_net.*`)

### `_net.fetch`
- **Inputs**: `url` (string), `method` (string), `headers` (map)
- **Description**: Performs an HTTP request (GET/POST). Used for health checks or artifact retrieval.
- **Probe**: returns status code and response body (restricted size).

---

## 5. Why Irreducible?
A built-in is considered irreducible if:
1. It interacts directly with the OS/Environment via non-compositional syscalls.
2. It provides the "atoms" of state change that all higher-level primitives eventually expand into.
