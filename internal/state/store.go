// Package state implements the SQLite-backed append-only execution state store.
package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"devopsctl/internal/proto"
)

const schema = `
CREATE TABLE IF NOT EXISTS executions (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	node_id        TEXT    NOT NULL,
	target         TEXT    NOT NULL,
	plan_hash      TEXT    NOT NULL DEFAULT '',
	content_hash   TEXT    NOT NULL,
	timestamp      INTEGER NOT NULL,
	status         TEXT    NOT NULL,  -- "success" | "failed" | "partial" | "rolled_back"
	changeset_json TEXT    NOT NULL,
	inputs_json    TEXT    NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_executions_node_target ON executions(node_id, target);
`

// Store manages the local SQLite state database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the state database at ~/.devopsctl/state.db.
func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".devopsctl")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dir, "state.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}
	// Add plan_hash column for existing databases (fails silently if it already exists)
	_, _ = db.Exec(`ALTER TABLE executions ADD COLUMN plan_hash TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE executions ADD COLUMN inputs_json TEXT NOT NULL DEFAULT '{}'`)
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Record appends one execution record (append-only).
func (s *Store) Record(nodeID, target, planHash, contentHash, status string, cs proto.ChangeSet, inputs map[string]any) error {
	csJSON, err := json.Marshal(cs)
	if err != nil {
		return err
	}
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO executions (node_id, target, plan_hash, content_hash, timestamp, status, changeset_json, inputs_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		nodeID, target, planHash, contentHash, time.Now().Unix(), status, string(csJSON), string(inputsJSON),
	)
	return err
}

// Execution is a row from the executions table.
type Execution struct {
	ID          int64
	NodeID      string
	Target      string
	PlanHash    string
	ContentHash string
	Timestamp   time.Time
	Status      string
	ChangeSet   proto.ChangeSet
	Inputs      map[string]any
}

// LastSuccessful returns the most recent "applied" execution for a node+target,
// used to determine if the current state matches what was last applied.
func (s *Store) LastSuccessful(nodeID, target string) (*Execution, error) {
	row := s.db.QueryRow(
		`SELECT id, plan_hash, content_hash, timestamp, changeset_json, inputs_json
		 FROM executions
		 WHERE node_id = ? AND target = ? AND status = 'success'
		 ORDER BY id DESC LIMIT 1`,
		nodeID, target,
	)
	var e Execution
	var ts int64
	var csJSON, inputsJSON string
	e.NodeID = nodeID
	e.Target = target
	if err := row.Scan(&e.ID, &e.PlanHash, &e.ContentHash, &ts, &csJSON, &inputsJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	e.Timestamp = time.Unix(ts, 0)
	if err := json.Unmarshal([]byte(csJSON), &e.ChangeSet); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(inputsJSON), &e.Inputs); err != nil {
		return nil, err
	}
	return &e, nil
}

// List returns all executions for a node (most recent first).
func (s *Store) List(nodeID string) ([]Execution, error) {
	rows, err := s.db.Query(
		`SELECT id, node_id, target, plan_hash, content_hash, timestamp, status, changeset_json, inputs_json
		 FROM executions WHERE node_id = ? ORDER BY id DESC`,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Execution
	for rows.Next() {
		var e Execution
		var ts int64
		var csJSON, inputsJSON string
		if err := rows.Scan(&e.ID, &e.NodeID, &e.Target, &e.PlanHash, &e.ContentHash, &ts, &e.Status, &csJSON, &inputsJSON); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0)
		_ = json.Unmarshal([]byte(csJSON), &e.ChangeSet)
		_ = json.Unmarshal([]byte(inputsJSON), &e.Inputs)
		result = append(result, e)
	}
	return result, rows.Err()
}

// LastRun returns all executions associated with the most recent plan_hash.
func (s *Store) LastRun() ([]Execution, error) {
	var planHash string
	err := s.db.QueryRow(`SELECT plan_hash FROM executions ORDER BY id DESC LIMIT 1`).Scan(&planHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	
	rows, err := s.db.Query(
		`SELECT id, node_id, target, plan_hash, content_hash, timestamp, status, changeset_json, inputs_json
		 FROM executions WHERE plan_hash = ? ORDER BY id DESC`,
		planHash,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Execution
	for rows.Next() {
		var e Execution
		var ts int64
		var csJSON, inputsJSON string
		if err := rows.Scan(&e.ID, &e.NodeID, &e.Target, &e.PlanHash, &e.ContentHash, &ts, &e.Status, &csJSON, &inputsJSON); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0)
		_ = json.Unmarshal([]byte(csJSON), &e.ChangeSet)
		_ = json.Unmarshal([]byte(inputsJSON), &e.Inputs)
		result = append(result, e)
	}
	return result, rows.Err()
}
