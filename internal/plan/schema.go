// Package plan handles loading and validating execution plan JSON files.
package plan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
)

// Plan is the top-level execution plan.
type Plan struct {
	Version string   `json:"version"`
	Targets []Target `json:"targets"`
	Nodes   []Node   `json:"nodes"`
}

// Target is a remote server the controller can dispatch to.
type Target struct {
	ID      string            `json:"id"`
	Address string            `json:"address"` // host:port, e.g. "10.0.0.10:7700"
	Labels  map[string]string `json:"labels,omitempty"`
}

// RetryConfig holds retry parameters for a node (v0.8+).
type RetryConfig struct {
	Attempts int    `json:"attempts"`
	Delay    string `json:"delay"`
}

// Node is a single unit of work in the plan.
type Node struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Targets       []string       `json:"targets"` // Target IDs
	DependsOn     []string       `json:"depends_on,omitempty"`
	When          *WhenCondition `json:"when,omitempty"`
	FailurePolicy string         `json:"failure_policy,omitempty"`
	Inputs        map[string]any `json:"inputs"`

	// v0.8+ node contracts
	Idempotent  bool         `json:"idempotent,omitempty"`
	SideEffects string       `json:"side_effects,omitempty"` // "none" | "local" | "external"
	Retry       *RetryConfig `json:"retry,omitempty"`
	RollbackCmd []string     `json:"rollback_cmd,omitempty"`

	// v0.9 secret injection: keys that must be resolved from a secret provider at apply-time.
	// Inputs that depend on secrets are stored as "[SECRET:KEY]" sentinel placeholders in Inputs
	// and listed here so the controller knows which inputs to resolve before execution.
	RequiresSecrets []string `json:"requires_secrets,omitempty"`

	// v1.3+ probe and desired state for primitives
	Probe   map[string]any `json:"probe,omitempty"`   // Serialized probe expressions
	Desired map[string]any `json:"desired,omitempty"` // Serialized desired state expressions
}

// WhenCondition represents conditional execution rules.
type WhenCondition struct {
	Node    string `json:"node"`
	Changed bool   `json:"changed"`
}

// Load reads a JSON plan file from disk.
func Load(path string) (*Plan, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading plan file: %w", err)
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, nil, fmt.Errorf("parsing plan JSON: %w", err)
	}
	return &p, data, nil
}

// Hash deterministically hashes the node's definition and target to uniquely identify
// this specific unit of execution within the plan.
func (n *Node) Hash(targetID string) string {
	type hashStruct struct {
		Type        string         `json:"type"`
		Target      string         `json:"target"`
		Inputs      map[string]any `json:"inputs"`
		Idempotent  bool           `json:"idempotent,omitempty"`
		SideEffects string         `json:"side_effects,omitempty"`
		Retry       *RetryConfig   `json:"retry,omitempty"`
		RollbackCmd []string       `json:"rollback_cmd,omitempty"`
	}

	hs := hashStruct{
		Type:        n.Type,
		Target:      targetID,
		Inputs:      n.Inputs,
		Idempotent:  n.Idempotent,
		SideEffects: n.SideEffects,
		Retry:       n.Retry,
		RollbackCmd: n.RollbackCmd,
	}

	// json.Marshal reliably orders map keys
	data, err := json.Marshal(hs)
	if err != nil {
		panic(fmt.Sprintf("failed to hash node %s: %v", n.ID, err)) // should not happen with basic JSON types
	}

	return fmt.Sprintf("%x", sha256.Sum256(data))
}
