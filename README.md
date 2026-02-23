# devopsctl

**A programming-first DevOps automation tool with deterministic execution and state management.**

devopsctl is a powerful infrastructure-as-code tool that compiles high-level `.devops` plans into flat, deterministic primitives for distributed execution. It provides idempotent operations, dependency resolution, parallel execution, and comprehensive state tracking.

## Table of Contents

- [Features](#features)
- [Core Concepts](#core-concepts)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Writing Plans](#writing-plans)
  - [CLI Commands](#cli-commands)
- [Language Versions](#language-versions)
- [Examples](#examples)
- [Architecture](#architecture)
- [Development](#development)

## Features

- **Declarative Language**: Write infrastructure plans in the `.devops` language
- **Deterministic Compilation**: All high-level constructs compile to flat primitives
- **Dependency Resolution**: Automatic dependency graph construction and execution
- **Parallel Execution**: Concurrent node execution with configurable parallelism
- **State Management**: Built-in state tracking for idempotent operations
- **Reconciliation**: Detect and correct infrastructure drift automatically
- **Resume Capability**: Resume failed executions from the last successful state
- **Dry-Run Mode**: Preview changes before applying them
- **Rollback Support**: Reverse previous executions safely
- **Distributed Execution**: Agent-based architecture for remote target management

## Core Concepts

- **Target**: A remote machine or environment where operations execute (identified by address)
- **Node**: A unit of work with a specific primitive type (e.g., `file.sync`, `process.exec`)
- **Primitive**: Built-in operation types (file synchronization, process execution)
- **Plan**: A collection of targets and nodes that define the desired state
- **State Store**: Local SQLite database tracking execution history
- **Agent**: Daemon running on target machines to execute primitives

## Installation

### Prerequisites

- Go 1.18 or higher
- Linux, macOS, or Windows

### Build from Source

```bash
# Clone the repository
git clone https://github.com/ahmednour1430464/dotdevops.git
cd devopsctl

# Build the binary
go build -o devopsctl ./cmd/devopsctl

# (Optional) Move to PATH
sudo mv devopsctl /usr/local/bin/
```

### Verify Installation

```bash
devopsctl --version
```

## Quick Start

### 1. Start the Agent

First, start the devopsctl agent on the target machine (or localhost for testing):

```bash
devopsctl agent --addr 127.0.0.1:7700
```

The agent runs in the foreground. Keep this terminal open or run it as a background service.

### 2. Create Your First Plan

Create a file named `plan.devops`:

```hcl
target "local" {
  address = "127.0.0.1:7700"
}

node "hello" {
  type    = process.exec
  targets = [local]
  
  cmd = ["echo", "Hello from devopsctl!"]
  cwd = "/tmp"
}
```

### 3. Apply the Plan

```bash
# Compile and apply the plan
devopsctl apply plan.devops

# Or preview changes first
devopsctl apply --dry-run plan.devops
```

### 4. Check State

```bash
# View execution history
devopsctl state list
```

## Usage

### Writing Plans

Plans are written in the `.devops` language. Here's the basic structure:

#### Targets

Define where operations will execute:

```hcl
target "production" {
  address = "192.168.1.100:7700"
}

target "staging" {
  address = "192.168.1.101:7700"
}
```

#### Nodes

Define operations to perform:

```hcl
# File synchronization
node "deploy-app" {
  type    = file.sync
  targets = [production]
  
  src  = "./build"
  dest = "/var/www/myapp"
}

# Process execution
node "restart-service" {
  type       = process.exec
  targets    = [production]
  depends_on = ["deploy-app"]
  
  cmd = ["systemctl", "restart", "myapp"]
  cwd = "/var/www/myapp"
}
```

#### Variables (Let Bindings)

Use variables for reusable values:

```hcl
let app_name = "myapp"
let base_dir = "/var/www"
let full_path = base_dir + "/" + app_name

node "deploy" {
  type    = file.sync
  targets = [local]
  src     = "./build"
  dest    = full_path
}
```

#### Expressions (v0.3+)

Use expressions for dynamic values:

```hcl
let is_prod = true
let log_level = is_prod ? "error" : "debug"
let backup_enabled = is_prod && true
let deploy_path = is_prod ? "/var/www/prod" : "/var/www/dev"

node "configure" {
  type    = process.exec
  targets = [local]
  cmd     = ["echo", log_level]
}
```

### CLI Commands

#### `devopsctl apply`

Execute a plan against configured targets.

```bash
# Apply a .devops plan (auto-compiles)
devopsctl apply plan.devops

# Apply a compiled JSON plan
devopsctl apply plan.json

# Preview changes without applying
devopsctl apply --dry-run plan.devops

# Control parallelism
devopsctl apply --parallelism 5 plan.devops

# Resume a failed execution
devopsctl apply --resume plan.devops

# Specify language version
devopsctl apply --lang v0.6 plan.devops
```

**Flags:**
- `--dry-run`: Show changes without applying
- `--parallelism N`: Max concurrent node executions (default: 10)
- `--resume`: Resume from last failed execution
- `--lang VERSION`: Language version (v0.1, v0.2, v0.3, v0.4, v0.5, v0.6)

#### `devopsctl reconcile`

Detect and correct infrastructure drift using recorded state as truth.

```bash
# Reconcile infrastructure to match plan
devopsctl reconcile plan.devops

# Preview drift without correcting
devopsctl reconcile --dry-run plan.devops

# Reconcile with controlled parallelism
devopsctl reconcile --parallelism 5 plan.json
```

**Flags:**
- `--dry-run`: Preview drift without applying changes
- `--parallelism N`: Max concurrent node executions (default: 10)
- `--lang VERSION`: Language version for .devops files

#### `devopsctl agent`

Start the agent daemon on a target machine.

```bash
# Start agent on default port
devopsctl agent

# Start agent on specific address
devopsctl agent --addr 0.0.0.0:7700

# Start agent on custom port
devopsctl agent --addr :8080
```

**Flags:**
- `--addr ADDRESS`: Listen address (default: `:7700`)

#### `devopsctl state`

Inspect execution history and state.

```bash
# List all executions
devopsctl state list

# Filter by node ID
devopsctl state list --node deploy-app

# Show detailed execution information
devopsctl state list
```

**Flags:**
- `--node ID`: Filter by node ID

#### `devopsctl plan`

Manage plan files.

```bash
# Compile .devops to JSON
devopsctl plan build plan.devops

# Save compiled plan to file
devopsctl plan build plan.devops --output plan.json

# Compute plan fingerprint (hash)
devopsctl plan hash plan.json
```

**Flags:**
- `--output FILE`: Write compiled plan to file
- `--lang VERSION`: Language version for compilation

#### `devopsctl rollback`

Reverse previous executions.

```bash
# Rollback last execution
devopsctl rollback --last
```

**Flags:**
- `--last`: Rollback the most recent execution

## Language Versions

devopsctl supports multiple language versions with incremental features:

| Version | Features | Status |
|---------|----------|--------|
| v0.1 | Targets, Nodes, Primitives | ✅ Stable |
| v0.2 | Let bindings (variables) | ✅ Stable |
| v0.3 | Expressions (ternary, operators, concat) | ✅ Stable |
| v0.4 | Reusable steps (macros) | ✅ Stable |
| v0.5 | Nested steps, For-loops | 🔧 In Development |
| v0.6 | Step parameters | 🔧 In Development |
| v0.7 | Step libraries (imports) | 🔧 Planned |

**Default version**: v0.3

Specify version with `--lang` flag:
```bash
devopsctl apply --lang v0.4 plan.devops
```

## Examples

### Basic File Synchronization

```hcl
target "webserver" {
  address = "192.168.1.100:7700"
}

node "sync-website" {
  type    = file.sync
  targets = [webserver]
  
  src  = "./dist"
  dest = "/var/www/html"
}
```

### Multi-Step Deployment with Dependencies

```hcl
target "app-server" {
  address = "10.0.1.50:7700"
}

node "deploy-code" {
  type    = file.sync
  targets = [app-server]
  src     = "./build"
  dest    = "/opt/myapp"
}

node "install-deps" {
  type       = process.exec
  targets    = [app-server]
  depends_on = ["deploy-code"]
  cmd        = ["npm", "install", "--production"]
  cwd        = "/opt/myapp"
}

node "restart-app" {
  type       = process.exec
  targets    = [app-server]
  depends_on = ["install-deps"]
  cmd        = ["systemctl", "restart", "myapp"]
}
```

### Environment-Specific Configuration

```hcl
target "prod" {
  address = "prod.example.com:7700"
}

target "dev" {
  address = "dev.example.com:7700"
}

let is_production = true
let app_dir = is_production ? "/var/www/prod" : "/var/www/dev"
let config_file = is_production ? "config.prod.json" : "config.dev.json"

node "deploy" {
  type    = file.sync
  targets = is_production ? [prod] : [dev]
  src     = "./dist"
  dest    = app_dir
}

node "configure" {
  type       = process.exec
  targets    = is_production ? [prod] : [dev]
  depends_on = ["deploy"]
  cmd        = ["cp", config_file, "config.json"]
  cwd        = app_dir
}
```

### Parallel Multi-Target Deployment

```hcl
target "web1" {
  address = "web1.example.com:7700"
}

target "web2" {
  address = "web2.example.com:7700"
}

target "web3" {
  address = "web3.example.com:7700"
}

node "deploy-all" {
  type    = file.sync
  targets = [web1, web2, web3]
  src     = "./dist"
  dest    = "/var/www/html"
}
```

## Execution Contexts

Execution contexts define the security boundaries for primitive execution on agent machines. They specify who executes operations, with what privileges, on which resources, and how execution is audited.

### What Are Execution Contexts?

An execution context is a security envelope that answers:

- **Who executes**: User and group identity
- **With what privileges**: Escalation rules (sudo/runuser)
- **On which resources**: Filesystem path restrictions
- **Under which restrictions**: Process controls, network access
- **How is it audited**: Logging verbosity and requirements

Contexts are:
- **Agent-owned**: Configured on the agent, not in DSL
- **Primitive-bound**: Automatically selected based on primitive type
- **Security-enforced**: All validations happen before execution
- **Fully audited**: Every execution logged with context metadata

### Configuration

Create a contexts YAML file with one or more execution contexts:

```yaml
contexts:
  - name: safe_user_space
    purpose: Safe operations without privileges
    trust_level: low
    identity:
      user: devopsctl
      group: devopsctl
    privilege:
      allow_escalation: false
    filesystem:
      readable_paths:
        - /tmp
        - /home/devopsctl
      writable_paths:
        - /tmp/devopsctl
        - /home/devopsctl/work
      denied_paths:
        - /etc
        - /usr/bin
    process:
      denied_executables:
        - rm
        - dd
      resource_limits:
        max_memory_mb: 512
        max_processes: 10
    network:
      allow_network: false
      scope: none
    audit:
      level: standard
      log_stdout: true
      log_stderr: true
```

Start the agent with the contexts configuration:

```bash
devopsctl agent --addr :7700 --contexts /etc/devopsctl/contexts.yaml
```

### Example Configurations

Example context configurations are provided in [`examples/contexts/`](examples/contexts/):

- **minimal.yaml**: Single safe user context for basic operations
- **multi-tier.yaml**: Low/medium/high trust contexts for different security levels
- **production.yaml**: Hardened production context with strict restrictions

### Context Properties

#### Identity
- `user`: Execution username (required)
- `group`: Primary group
- `groups`: Supplementary groups

#### Privilege
- `allow_escalation`: Enable sudo/privilege escalation
- `sudo_commands`: Allowed commands when escalation is enabled
- `no_password`: Use NOPASSWD sudo (security risk)

#### Filesystem
- `readable_paths`: Paths that can be read
- `writable_paths`: Paths that can be written
- `denied_paths`: Explicitly denied paths (highest priority)

#### Process
- `allowed_executables`: Whitelist of executables (empty = all allowed)
- `denied_executables`: Blacklist of executables
- `environment`: Enforced environment variables
- `resource_limits`: Memory, CPU, and process limits

#### Network
- `allow_network`: Enable/disable network access
- `allowed_ports`: Specific port restrictions
- `scope`: "none", "internal", or "full"

#### Audit
- `level`: "minimal", "standard", or "full"
- `log_stdout`: Log command stdout
- `log_stderr`: Log command stderr
- `log_env`: Log environment variables

### Audit Logging

Audit logs are written in JSON lines format to the configured audit log file (default: `/var/log/devopsctl-audit.log`). Each execution produces a structured audit entry with:

- Timestamp, node ID, primitive type
- Context name, execution user, trust level
- Command details (based on audit level)
- Exit code, status (success/failed/denied)
- Stdout/stderr (if configured)
- Execution duration
- Error messages (if any)

Audit logs can be analyzed using standard JSON processing tools:

```bash
# View recent executions
tail -f /var/log/devopsctl-audit.log | jq .

# Find failed executions
jq 'select(.status == "failed")' /var/log/devopsctl-audit.log

# List all contexts used
jq -r '.context_name' /var/log/devopsctl-audit.log | sort | uniq
```

### Security Best Practices

1. **Principle of Least Privilege**: Use the lowest trust level and most restrictive context for each operation
2. **Explicit Deny**: Use `denied_paths` and `denied_executables` to explicitly block dangerous operations
3. **Audit Everything**: Use `audit.level: full` in production for complete traceability
4. **Validate Contexts**: Test context configurations thoroughly before deploying to production
5. **Monitor Audit Logs**: Regularly review audit logs for suspicious activity or denied operations

## Architecture

devopsctl follows a compile-to-primitives architecture:

```
.devops Source → Parser → AST → Validator → Lowering → Flat Plan (JSON)
                                                            ↓
                                                      Orchestrator
                                                            ↓
                                              Dependency Graph Builder
                                                            ↓
                                                  Parallel Executor
                                                            ↓
                                              Agent Communication
                                                            ↓
                                            Primitives (file.sync, process.exec)
```

### Key Principles

1. **All language features compile to flat primitives** - No high-level constructs survive compilation
2. **Hashes are computed after full expansion** - Ensures deterministic builds
3. **Deterministic order everywhere** - Reproducible across environments
4. **Validation is version-strict** - Explicit feature gates per version

### Components

- **Compiler** (`internal/devlang/`): Lexer, parser, AST, validator, lowering
- **Plan Schema** (`internal/plan/`): JSON plan structure and validation
- **Controller** (`internal/controller/`): Orchestrator, graph builder, execution engine
- **Primitives** (`internal/primitive/`): File sync, process execution
- **State Store** (`internal/state/`): SQLite-based execution tracking
- **Agent** (`internal/agent/`): Remote execution daemon and protocol
- **CLI** (`cmd/devopsctl/`): Command-line interface

## Development

### Running Tests

```bash
# Unit tests
go test ./...

# Language version tests
./test_v0_3.sh
./test_v0_4.sh
./test_v0_5.sh
./test_v0_6.sh

# Hash stability tests
./test_hash_stability.sh

# End-to-end tests
./test_e2e.sh
```

### Project Structure

```
devopsctl/
├── cmd/devopsctl/          # CLI entry point
├── internal/
│   ├── agent/              # Agent server and handler
│   ├── controller/         # Orchestrator and execution engine
│   ├── devlang/            # Language compiler (lexer, parser, AST)
│   ├── plan/               # Plan schema and validation
│   ├── primitive/          # Built-in primitives
│   ├── proto/              # Protocol messages
│   └── state/              # State store implementation
├── tests/                  # Language version tests
│   ├── v0_3/
│   ├── v0_4/
│   ├── v0_5/
│   └── v0_6/
├── DESIGN.md               # Architecture principles
├── LANGUAGE_VERSIONS.md    # Version feature matrix
└── README.md               # This file
```

### Contributing

Contributions are welcome! Please follow the design principles documented in [DESIGN.md](DESIGN.md).

## License

[Add your license information here]

## Support

For issues, questions, or contributions, please visit the project repository.
