// Package plan handles loading and validating execution plan JSON files.
package plan

import (
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
	ID      string `json:"id"`
	Address string `json:"address"` // host:port, e.g. "10.0.0.10:7700"
}

// Node is a single unit of work in the plan.
type Node struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Targets []string       `json:"targets"` // Target IDs
	Inputs  map[string]any `json:"inputs"`
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
