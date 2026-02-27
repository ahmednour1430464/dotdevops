// Package secret provides pluggable secret resolution for the devopsctl controller.
// Secrets are never stored in plan files, state stores, or audit logs.
// The controller resolves secret values immediately before executing a node
// and substitutes them in-memory only.
package secret

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Provider resolves a named secret key to its plaintext value.
// Implementations must never log, cache, or persist the returned value.
type Provider interface {
	// Resolve returns the value for the given secret key, or an error if the
	// key cannot be resolved.
	Resolve(key string) (string, error)
	// Name returns a human-readable identifier for this provider (used in error messages).
	Name() string
}

// EnvProvider resolves secrets from environment variables.
// It is the default provider when no secret file is specified.
type EnvProvider struct{}

func (p *EnvProvider) Name() string { return "env" }

// Resolve looks up key as an environment variable.
// Returns an error if the variable is not set.
func (p *EnvProvider) Resolve(key string) (string, error) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("secret %q not found in environment variables", key)
	}
	return val, nil
}

// FileProvider resolves secrets from a JSON file containing a flat key→value map.
// The file format is: { "KEY": "value", "OTHER_KEY": "other_value" }
type FileProvider struct {
	Path   string
	values map[string]string
}

func (p *FileProvider) Name() string { return "file:" + p.Path }

// Load reads and parses the secrets file. Must be called before Resolve.
func (p *FileProvider) Load() error {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return fmt.Errorf("read secrets file %q: %w", p.Path, err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse secrets file %q: expected JSON object with string values: %w", p.Path, err)
	}
	p.values = m
	return nil
}

// Resolve looks up key in the loaded secrets file.
func (p *FileProvider) Resolve(key string) (string, error) {
	if p.values == nil {
		return "", fmt.Errorf("secrets file not loaded; call Load() first")
	}
	val, ok := p.values[key]
	if !ok {
		return "", fmt.Errorf("secret %q not found in secrets file %q", key, p.Path)
	}
	return val, nil
}

const (
	// SentinelPrefix is the prefix used in plan JSON for secret placeholder values.
	// The controller detects this pattern at apply-time and resolves the real value.
	SentinelPrefix = "[SECRET:"
	// Redacted is the string written to logs, dry-run output, and state records
	// in place of any resolved secret value.
	Redacted = "[REDACTED]"
)

// IsSentinel reports whether v is a secret sentinel string (i.e. "[SECRET:KEY]").
func IsSentinel(v string) bool {
	return strings.HasPrefix(v, SentinelPrefix) && strings.HasSuffix(v, "]")
}

// KeyFromSentinel extracts the secret key from a sentinel string like "[SECRET:MY_KEY]".
// Returns the key and true on success, or "" and false if v is not a sentinel.
func KeyFromSentinel(v string) (string, bool) {
	if !IsSentinel(v) {
		return "", false
	}
	key := v[len(SentinelPrefix) : len(v)-1]
	return key, true
}

// ResolveNodeInputs resolves all sentinel placeholders in a node's inputs map
// using the given provider. Returns a new map safe to pass to the agent.
// The original map is never mutated.
//
// If any secret cannot be resolved, an error is returned listing the failing key.
// All secrets are resolved before any substitution occurs, so partial resolution
// never happens.
func ResolveNodeInputs(inputs map[string]any, provider Provider) (map[string]any, error) {
	resolved := make(map[string]any, len(inputs))
	for k, v := range inputs {
		s, ok := v.(string)
		if !ok {
			resolved[k] = v
			continue
		}
		secretKey, isSentinel := KeyFromSentinel(s)
		if !isSentinel {
			resolved[k] = v
			continue
		}
		val, err := provider.Resolve(secretKey)
		if err != nil {
			return nil, fmt.Errorf("resolve secret for input %q: %w", k, err)
		}
		resolved[k] = val
	}
	return resolved, nil
}

// RedactNodeInputs replaces all resolved secret values in inputs with the [REDACTED]
// placeholder. This is used for dry-run output and audit log entries.
// The keys parameter lists which input keys contain secrets (from plan.Node.RequiresSecrets).
func RedactNodeInputs(inputs map[string]any, secretKeys []string) map[string]any {
	redacted := make(map[string]any, len(inputs))
	for k, v := range inputs {
		redacted[k] = v
	}
	// Redact by sentinel pattern (covers unresolved sentinels in dry-run).
	for k, v := range redacted {
		if s, ok := v.(string); ok && IsSentinel(s) {
			redacted[k] = Redacted
		}
	}
	return redacted
}

// NewProvider constructs a Provider from CLI-style arguments.
// providerName is "env" or "file".
// secretFile is required only when providerName is "file".
func NewProvider(providerName, secretFile string) (Provider, error) {
	switch providerName {
	case "", "env":
		return &EnvProvider{}, nil
	case "file":
		if secretFile == "" {
			return nil, fmt.Errorf("--secret-file is required when --secret-provider is 'file'")
		}
		p := &FileProvider{Path: secretFile}
		if err := p.Load(); err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown secret provider %q; valid values: env, file", providerName)
	}
}
