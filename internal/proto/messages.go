// Package proto defines the line-delimited JSON wire protocol between
// the controller and agent.
package proto

// ---- Envelope ----

// Message is the top-level envelope. Every line on the wire is one JSON object
// with at minimum a "type" field. Callers unmarshal into Message first, then
// re-unmarshal the raw payload into the specific request/response struct.
type Message struct {
	Type string `json:"type"`
}

// ---- Controller → Agent ----

// DetectReq asks the agent to walk the destination directory and return its
// current file-tree state.
type DetectReq struct {
	Type      string         `json:"type"` // "detect_req"
	NodeID    string         `json:"node_id"`
	Primitive string         `json:"primitive"`
	Inputs    map[string]any `json:"inputs"`
}

// ApplyReq instructs the agent to apply a diff. File chunks follow
// immediately on the same connection as ChunkMsg lines.
type ApplyReq struct {
	Type      string    `json:"type"` // "apply_req"
	NodeID    string    `json:"node_id"`
	Primitive string    `json:"primitive"`
	PlanHash  string    `json:"plan_hash"`
	ChangeSet ChangeSet `json:"changeset"`
}

// RollbackReq asks the agent to undo the last apply for the given node.
type RollbackReq struct {
	Type        string   `json:"type"` // "rollback_req"
	NodeID      string   `json:"node_id"`
	Primitive   string   `json:"primitive"`
	PlanHash    string   `json:"plan_hash"`
	RollbackCmd []string `json:"rollback_cmd,omitempty"`
}

// ProbeReq asks the agent to evaluate probe expressions and return observed state (v1.3+).
type ProbeReq struct {
	Type      string         `json:"type"` // "probe_req"
	NodeID    string         `json:"node_id"`
	Primitive string         `json:"primitive"`
	Probe     map[string]any `json:"probe"` // Field name -> expression to evaluate
}

// ChunkMsg carries one fragment of a file being streamed to the agent.
type ChunkMsg struct {
	Type string `json:"type"` // "chunk"
	Path string `json:"path"` // relative path inside dest
	Data []byte `json:"data"` // raw bytes (JSON base64 automatically)
	EOF  bool   `json:"eof"`  // true on last chunk for this path
}

// ---- Agent → Controller ----

// DetectResp is the agent's reply to DetectReq.
type DetectResp struct {
	Type   string   `json:"type"` // "detect_resp"
	NodeID string   `json:"node_id"`
	State  FileTree `json:"state"`
	Error  string   `json:"error,omitempty"`
}

// ApplyResp is the agent's reply to ApplyReq.
type ApplyResp struct {
	Type   string `json:"type"` // "apply_resp"
	NodeID string `json:"node_id"`
	Result Result `json:"result"`
	Error  string `json:"error,omitempty"`
}

// RollbackResp is the agent's reply to RollbackReq.
type RollbackResp struct {
	Type   string `json:"type"` // "rollback_resp"
	NodeID string `json:"node_id"`
	Result Result `json:"result"`
	Error  string `json:"error,omitempty"`
}

// ProbeResp is the agent's reply to ProbeReq (v1.3+).
type ProbeResp struct {
	Type   string         `json:"type"` // "probe_resp"
	NodeID string         `json:"node_id"`
	State  map[string]any `json:"state"` // Observed values keyed by field name
	Error  string         `json:"error,omitempty"`
}

// ---- Shared data structures ----

// FileMeta holds all metadata for a single file.
type FileMeta struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	UID    int    `json:"uid"`
	GID    int    `json:"gid"`
	IsDir  bool   `json:"is_dir"`
}

// FileTree maps relative paths to their metadata.
type FileTree map[string]FileMeta

// ChangeSet describes the delta between source and destination.
type ChangeSet struct {
	Create []string `json:"create"` // paths to create (new files)
	Update []string `json:"update"` // paths to overwrite (changed files)
	Delete []string `json:"delete"` // paths to remove (delete_extra only)
	Chmod  []string `json:"chmod"`  // paths needing permission change
	Chown  []string `json:"chown"`  // paths needing ownership change
	Mkdir  []string `json:"mkdir"`  // directories to create
}

// Result is a structured outcome from apply or rollback.
type Result struct {
	Status       string `json:"status"`          // "success", "failed", "partial"
	Class        string `json:"class,omitempty"` // e.g., "transient", "fatal"
	ExitCode     int    `json:"exit_code,omitempty"`
	Stdout       string `json:"stdout,omitempty"`
	Stderr       string `json:"stderr,omitempty"`
	RollbackSafe bool   `json:"rollback_safe"`

	// Legacy capabilities used by filesync
	Message string   `json:"message,omitempty"`
	Applied []string `json:"applied,omitempty"`
	Failed  []string `json:"failed,omitempty"`
}
