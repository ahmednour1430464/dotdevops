// Package controller implements the orchestrator that connects to agents,
// runs the detect-diff-apply cycle, and manages state.
package controller

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"devopsctl/internal/plan"
	"devopsctl/internal/primitive/filesync"
	"devopsctl/internal/proto"
	"devopsctl/internal/state"
)

const defaultAgentPort = "7700"

// RunOptions configures a controller execution run.
type RunOptions struct {
	DryRun      bool
	Parallelism int // 0 = unlimited
}

// Run executes a plan file end-to-end.
func Run(p *plan.Plan, rawPlan []byte, store *state.Store, opts RunOptions) error {
	planHash := fmt.Sprintf("%x", sha256.Sum256(rawPlan))
	if opts.Parallelism <= 0 {
		opts.Parallelism = 10
	}

	// Build target index.
	targetMap := make(map[string]plan.Target, len(p.Targets))
	for _, t := range p.Targets {
		targetMap[t.ID] = t
	}

	// Semaphore for parallelism control.
	sem := make(chan struct{}, opts.Parallelism)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, node := range p.Nodes {
		node := node // capture
		for _, tID := range node.Targets {
			tID := tID
			target, ok := targetMap[tID]
			if !ok {
				continue
			}
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer func() { <-sem; wg.Done() }()
				if err := runNode(node, target, planHash, store, opts); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("[%s → %s] %w", node.ID, target.ID, err))
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", e)
		}
		return fmt.Errorf("%d node(s) failed", len(errs))
	}
	return nil
}

// runNode handles one (node × target) pair.
func runNode(node plan.Node, target plan.Target, planHash string, store *state.Store, opts RunOptions) error {
	addr := addressWithPort(target.Address)
	if node.Type == "file.sync" {
		return runFileSync(addr, node, target, planHash, store, opts)
	} else if node.Type == "process.exec" {
		return runProcessExec(addr, node, target, planHash, store, opts)
	}
	return fmt.Errorf("unsupported primitive type: %s", node.Type)
}

func runFileSync(addr string, node plan.Node, target plan.Target, planHash string, store *state.Store, opts RunOptions) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect to agent %s: %w", addr, err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)

	// ── Step 1: Detect remote state ──────────────────────────────────────────
	_ = enc.Encode(proto.DetectReq{
		Type:      "detect_req",
		NodeID:    node.ID,
		Primitive: node.Type,
		Inputs:    node.Inputs,
	})
	var detectResp proto.DetectResp
	if err := readJSON(r, &detectResp); err != nil {
		return fmt.Errorf("detect response: %w", err)
	}
	if detectResp.Error != "" {
		return fmt.Errorf("agent detect error: %s", detectResp.Error)
	}
	destTree := detectResp.State

	// ── Step 2: Build source tree (controller-local) ──────────────────────────
	src, _ := node.Inputs["src"].(string)
	srcTree, err := filesync.BuildSourceTree(src)
	if err != nil {
		return fmt.Errorf("building source tree: %w", err)
	}

	var deleteExtra bool
	switch v := node.Inputs["delete_extra"].(type) {
	case bool:
		deleteExtra = v
	case string:
		deleteExtra = v == "true"
	}
	desiredMode := uint32(0)
	desiredUID, desiredGID := -1, -1

	cs := filesync.Diff(srcTree, destTree, desiredMode, desiredUID, desiredGID, deleteExtra)

	// ── Step 3: Display diff ──────────────────────────────────────────────────
	PrintDiff(node.ID, target.ID, cs)

	if filesync.IsEmpty(cs) {
		fmt.Printf("[%s → %s] ✓ no changes\n", node.ID, target.ID)
		return nil
	}

	if opts.DryRun {
		fmt.Printf("[%s → %s] dry-run: %d change(s) would be applied\n",
			node.ID, target.ID, totalChanges(cs))
		return nil
	}

	// ── Step 4: Apply ─────────────────────────────────────────────────────────
	// Re-dial for apply (separates detect and apply connections cleanly).
	conn.Close()
	conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("re-connect for apply: %w", err)
	}
	defer conn.Close()
	r = bufio.NewReader(conn)
	enc = json.NewEncoder(conn)

	applyReq := applyReqFull{
		ApplyReq: proto.ApplyReq{
			Type:      "apply_req",
			NodeID:    node.ID,
			Primitive: node.Type,
			PlanHash:  planHash,
			ChangeSet: cs,
		},
		Inputs: node.Inputs,
	}
	if err := enc.Encode(applyReq); err != nil {
		return fmt.Errorf("sending apply_req: %w", err)
	}

	// Stream file chunks for paths that need transfer.
	needsTransfer := append(cs.Create, cs.Update...)
	if err := streamFiles(src, needsTransfer, enc); err != nil {
		return fmt.Errorf("streaming files: %w", err)
	}

	// Read apply response.
	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return fmt.Errorf("agent apply error: %s", applyResp.Error)
	}

	res := applyResp.Result
	fmt.Printf("[%s → %s] %s\n", node.ID, target.ID, res.Message)

	// ── Step 5: Persist state ──────────────────────────────────────────────────
	contentHash := hashChangeSet(cs)
	inputsWithAddr := make(map[string]any)
	for k, v := range node.Inputs {
		inputsWithAddr[k] = v
	}
	inputsWithAddr["__target_addr"] = target.Address
	if err := store.Record(node.ID, target.ID, planHash, contentHash, res.Status, cs, inputsWithAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: state record failed: %v\n", err)
	}

	// ── Step 6: Rollback on fatal failure ─────────────────────────────────────
	if res.Status == "failed" {
		if res.RollbackSafe {
			fmt.Printf("[%s → %s] triggering rollback…\n", node.ID, target.ID)
			if err := doRollback(addr, node, planHash, cs); err != nil {
				fmt.Fprintf(os.Stderr, "WARN: rollback error: %v\n", err)
			}
		}
		return fmt.Errorf("apply failed: %s", res.Message)
	}

	return nil
}

func runProcessExec(addr string, node plan.Node, target plan.Target, planHash string, store *state.Store, opts RunOptions) error {
	if opts.DryRun {
		cmdArr, _ := node.Inputs["cmd"].([]any)
		fmt.Printf("[%s → %s] dry-run: would execute %v\n", node.ID, target.ID, cmdArr)
		return nil
	}

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect to agent %s: %w", addr, err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)

	applyReq := applyReqFull{
		ApplyReq: proto.ApplyReq{
			Type:      "apply_req",
			NodeID:    node.ID,
			Primitive: node.Type,
			PlanHash:  planHash,
			ChangeSet: proto.ChangeSet{},
		},
		Inputs: node.Inputs,
	}
	if err := enc.Encode(applyReq); err != nil {
		return fmt.Errorf("sending apply_req: %w", err)
	}

	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return fmt.Errorf("agent apply error: %s", applyResp.Error)
	}

	res := applyResp.Result
	fmt.Printf("[%s → %s] process exited with code %d\n", node.ID, target.ID, res.ExitCode)
	if res.Stdout != "" {
		fmt.Printf("--- stdout ---\n%s\n--------------\n", res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Printf("--- stderr ---\n%s\n--------------\n", res.Stderr)
	}

	contentHash := "process_exec_no_cs"
	inputsWithAddr := make(map[string]any)
	for k, v := range node.Inputs {
		inputsWithAddr[k] = v
	}
	inputsWithAddr["__target_addr"] = target.Address
	if err := store.Record(node.ID, target.ID, planHash, contentHash, res.Status, proto.ChangeSet{}, inputsWithAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: state record failed: %v\n", err)
	}

	if res.Status == "failed" {
		return fmt.Errorf("process failed with exit code %d (class: %s)", res.ExitCode, res.Class)
	}

	return nil
}

// streamFiles sends file chunk messages for the given relative paths.
func streamFiles(srcRoot string, paths []string, enc *json.Encoder) error {
	const chunkSize = 256 * 1024
	buf := make([]byte, chunkSize)
	for _, rel := range paths {
		abs := filepath.Join(srcRoot, rel)
		f, err := os.Open(abs)
		if err != nil {
			return fmt.Errorf("open %s: %w", rel, err)
		}
		for {
			n, err := f.Read(buf)
			eof := err == io.EOF
			if n > 0 {
				chunk := proto.ChunkMsg{
					Type: "chunk",
					Path: rel,
					Data: append([]byte(nil), buf[:n]...),
					EOF:  eof || err != nil,
				}
				if encErr := enc.Encode(chunk); encErr != nil {
					f.Close()
					return encErr
				}
			}
			if eof {
				break
			}
			if err != nil {
				f.Close()
				return err
			}
		}
		f.Close()
	}
	// Send a sentinel EOF marker so the agent knows chunking is done.
	return enc.Encode(proto.ChunkMsg{Type: "chunk", EOF: true, Path: ""})
}

// doRollback sends a rollback_req to the agent.
func doRollback(addr string, node plan.Node, planHash string, cs proto.ChangeSet) error {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)

	type rollbackFull struct {
		proto.RollbackReq
		Inputs    map[string]any  `json:"inputs"`
		ChangeSet proto.ChangeSet `json:"changeset"`
	}
	if err := enc.Encode(rollbackFull{
		RollbackReq: proto.RollbackReq{
			Type:      "rollback_req",
			NodeID:    node.ID,
			Primitive: node.Type,
			PlanHash:  planHash,
		},
		Inputs:    node.Inputs,
		ChangeSet: cs,
	}); err != nil {
		return err
	}
	var resp proto.RollbackResp
	return readJSON(r, &resp)
}

// ── helpers ──────────────────────────────────────────────────────────────────

type applyReqFull struct {
	proto.ApplyReq
	Inputs map[string]any `json:"inputs"`
}

func readJSON(r *bufio.Reader, v interface{}) error {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}

func addressWithPort(addr string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr + ":" + defaultAgentPort
	}
	return addr
}

func totalChanges(cs proto.ChangeSet) int {
	return len(cs.Create) + len(cs.Update) + len(cs.Delete) +
		len(cs.Chmod) + len(cs.Chown) + len(cs.Mkdir)
}

func hashChangeSet(cs proto.ChangeSet) string {
	data, _ := json.Marshal(cs)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// RollbackLast fetches the most recent execution plan run and rolls back all successful file.sync nodes.
func RollbackLast(store *state.Store) error {
	execs, err := store.LastRun()
	if err != nil {
		return fmt.Errorf("fetch last run: %w", err)
	}
	if len(execs) == 0 {
		return fmt.Errorf("no previous run found to rollback")
	}

	for _, ex := range execs {
		if ex.Status != "success" && ex.Status != "partial" {
			continue
		}
		// Construct a dummy node to pass into doRollback
		node := plan.Node{
			ID:     ex.NodeID,
			Type:   "file.sync", // assume filesync for now, process.exec is not rollbackable yet
			Inputs: ex.Inputs,
		}
		
		addrStr, _ := ex.Inputs["__target_addr"].(string)
		if addrStr == "" {
			addrStr = ex.Target
		}
		addr := addressWithPort(addrStr)
		if err := doRollback(addr, node, ex.PlanHash, ex.ChangeSet); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: rollback failed for node %s on %s: %v\n", ex.NodeID, ex.Target, err)
		} else {
			fmt.Printf("[%s → %s] successfully rolled back\n", ex.NodeID, ex.Target)
			_ = store.Record(ex.NodeID, ex.Target, ex.PlanHash, "rollback_cs", "rolled_back", ex.ChangeSet, ex.Inputs)
		}
	}
	return nil
}
