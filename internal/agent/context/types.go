package context

// ExecutionContext defines the security and runtime envelope for primitive execution.
type ExecutionContext struct {
	Name       string           `yaml:"name" json:"name"`                 // Stable identifier (e.g., "safe_user_space")
	Purpose    string           `yaml:"purpose" json:"purpose"`           // Human-readable intent
	TrustLevel TrustLevel       `yaml:"trust_level" json:"trust_level"`   // Low / Medium / High
	Identity   IdentityConfig   `yaml:"identity" json:"identity"`         // User, group, UID/GID
	Privilege  PrivilegeConfig  `yaml:"privilege" json:"privilege"`       // Escalation rules
	Filesystem FilesystemConfig `yaml:"filesystem" json:"filesystem"`     // Path restrictions
	Process    ProcessConfig    `yaml:"process" json:"process"`           // Executable controls
	Network    NetworkConfig    `yaml:"network" json:"network"`           // Network access rules
	Audit      AuditConfig      `yaml:"audit" json:"audit"`               // Logging requirements
}

// TrustLevel defines the security trust level of an execution context.
type TrustLevel string

const (
	TrustLevelLow    TrustLevel = "low"    // Low privilege, restricted access
	TrustLevelMedium TrustLevel = "medium" // Moderate privilege
	TrustLevelHigh   TrustLevel = "high"   // High privilege, administrative
)

// IdentityConfig defines execution identity (user, group).
type IdentityConfig struct {
	User   string   `yaml:"user" json:"user"`     // Execution username (e.g., "devopsctl" or "root")
	Group  string   `yaml:"group" json:"group"`   // Primary group
	Groups []string `yaml:"groups" json:"groups"` // Supplementary groups
}

// PrivilegeConfig defines privilege escalation rules.
type PrivilegeConfig struct {
	AllowEscalation bool     `yaml:"allow_escalation" json:"allow_escalation"` // Can use sudo
	SudoCommands    []string `yaml:"sudo_commands" json:"sudo_commands"`       // Allowed commands (empty = all if escalation allowed)
	NoPassword      bool     `yaml:"no_password" json:"no_password"`           // NOPASSWD in sudoers (risky)
}

// FilesystemConfig defines filesystem access restrictions.
type FilesystemConfig struct {
	ReadOnlyPaths []string `yaml:"readable_paths" json:"readable_paths"` // Can only read
	WritablePaths []string `yaml:"writable_paths" json:"writable_paths"` // Can write
	DeniedPaths   []string `yaml:"denied_paths" json:"denied_paths"`     // Explicit deny (takes precedence)
}

// ProcessConfig defines process execution controls.
type ProcessConfig struct {
	AllowedExecutables []string          `yaml:"allowed_executables" json:"allowed_executables"` // Whitelist (empty = all allowed)
	DeniedExecutables  []string          `yaml:"denied_executables" json:"denied_executables"`   // Blacklist
	Environment        map[string]string `yaml:"environment" json:"environment"`                 // Enforced env vars
	ResourceLimits     ResourceLimits    `yaml:"resource_limits" json:"resource_limits"`         // Resource constraints
}

// ResourceLimits defines resource constraints for process execution.
type ResourceLimits struct {
	MaxMemoryMB   int `yaml:"max_memory_mb" json:"max_memory_mb"`     // Maximum memory in MB
	MaxCPUPercent int `yaml:"max_cpu_percent" json:"max_cpu_percent"` // Maximum CPU percentage
	MaxProcesses  int `yaml:"max_processes" json:"max_processes"`     // Maximum number of processes
}

// NetworkConfig defines network access controls.
type NetworkConfig struct {
	AllowNetwork bool   `yaml:"allow_network" json:"allow_network"` // Can access network at all
	AllowedPorts []int  `yaml:"allowed_ports" json:"allowed_ports"` // Specific ports if restricted
	Scope        string `yaml:"scope" json:"scope"`                 // "none", "internal", "full"
}

// AuditConfig defines audit and logging requirements.
type AuditConfig struct {
	Level     AuditLevel `yaml:"level" json:"level"`           // Minimal, Standard, Full
	LogStdout bool       `yaml:"log_stdout" json:"log_stdout"` // Log stdout
	LogStderr bool       `yaml:"log_stderr" json:"log_stderr"` // Log stderr
	LogEnv    bool       `yaml:"log_env" json:"log_env"`       // Log environment variables
}

// AuditLevel defines the verbosity of audit logging.
type AuditLevel string

const (
	AuditLevelMinimal  AuditLevel = "minimal"  // success/failure only
	AuditLevelStandard AuditLevel = "standard" // + inputs/outputs
	AuditLevelFull     AuditLevel = "full"     // + command, environment
)
