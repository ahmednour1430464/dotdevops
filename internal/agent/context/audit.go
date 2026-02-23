package context

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// AuditEntry represents a single audit log entry for primitive execution.
type AuditEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	NodeID        string            `json:"node_id"`
	PrimitiveType string            `json:"primitive_type"`
	ContextName   string            `json:"context_name"`
	ExecutionUser string            `json:"execution_user"`
	TrustLevel    TrustLevel        `json:"trust_level"`
	Command       []string          `json:"command,omitempty"`
	WorkingDir    string            `json:"working_dir,omitempty"`
	ExitCode      int               `json:"exit_code"`
	Status        string            `json:"status"` // "success", "failed", "denied"
	Stdout        string            `json:"stdout,omitempty"`
	Stderr        string            `json:"stderr,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	Duration      time.Duration     `json:"duration"`
	ErrorMessage  string            `json:"error_message,omitempty"`
}

// AuditLogger writes audit entries to a file in JSON lines format.
type AuditLogger struct {
	file  *os.File
	mutex sync.Mutex
}

// NewAuditLogger creates a new audit logger that writes to the specified file path.
// The file is opened in append mode and created if it doesn't exist.
func NewAuditLogger(path string) (*AuditLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &AuditLogger{file: f}, nil
}

// Log writes an audit entry to the log file as a JSON line.
// This method is thread-safe.
func (a *AuditLogger) Log(entry AuditEntry) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = a.file.Write(append(data, '\n'))
	return err
}

// Close closes the audit log file.
func (a *AuditLogger) Close() error {
	return a.file.Close()
}
