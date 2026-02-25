// Package controller implements the orchestrator that connects to agents,
// runs the detect-diff-apply cycle, and manages state.
package controller

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

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
	Resume      bool
	Reconcile   bool
	Observe     bool
	
	TLSCertPath string
	TLSKeyPath  string
	TLSCAPath   string
}

// Run executes a plan file end-to-end using the execution graph.
func Run(p *plan.Plan, rawPlan []byte, store *state.Store, opts RunOptions) error {
	planHash := fmt.Sprintf("%x", sha256.Sum256(rawPlan))
	if opts.Parallelism <= 0 {
		opts.Parallelism = 10
	}

	targetMap := make(map[string]plan.Target, len(p.Targets))
	for _, t := range p.Targets {
		targetMap[t.ID] = t
	}

	graph, err := BuildGraph(p.Nodes)
	if err != nil {
		return fmt.Errorf("building execution graph: %w", err)
	}

	var mu sync.Mutex
	var errs []error

	nodeStates := make(map[string]string)
	nodeChanged := make(map[string]bool)
	for id := range graph.Nodes {
		nodeStates[id] = "pending"
		nodeChanged[id] = false
	}

	inDegree := make(map[string]int)
	for id, deg := range graph.InDegree {
		inDegree[id] = deg
	}

	readyQueue := make(chan string, len(graph.Nodes))
	for id, deg := range inDegree {
		if deg == 0 {
			readyQueue <- id
		}
	}

	doneChan := make(chan string, len(graph.Nodes))
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	haltExecution := false

	// Target-level semaphore
	sem := make(chan struct{}, opts.Parallelism)

	// Worker dispatcher
	go func() {
		for id := range readyQueue {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()

				node := graph.Nodes[id]

				mu.Lock()
				if haltExecution {
					nodeStates[id] = "skipped"
					mu.Unlock()
					doneChan <- id
					return
				}

				cascadeSkip := false
				isBlocked := false
				for _, depID := range node.DependsOn {
					st := nodeStates[depID]
					if st == "failed" || st == "skipped" {
						cascadeSkip = true
						isBlocked = true
						break
					}
				}

				if !cascadeSkip && node.When != nil {
					whenCond := node.When
					depChanged := nodeChanged[whenCond.Node]
					if depChanged != whenCond.Changed {
						cascadeSkip = true
						fmt.Printf("[%s] skipped: condition (node %s changed == %v) not met\n", node.ID, whenCond.Node, whenCond.Changed)
					}
				}
				mu.Unlock()

				if cascadeSkip {
					mu.Lock()
					if isBlocked {
						nodeStates[id] = "skipped"
					} else {
						nodeStates[id] = "skipped"
					}
					recStatus := nodeStates[id]
					mu.Unlock()
					
					for _, tID := range node.Targets {
						if target, ok := targetMap[tID]; ok {
							inputsWithAddr := make(map[string]any)
							for k, v := range node.Inputs {
								inputsWithAddr[k] = v
							}
							inputsWithAddr["__target_addr"] = target.Address
							nodeHash := node.Hash(target.ID)
							_ = store.Record(node.ID, target.ID, node.Type, planHash, nodeHash, "skipped_cs", recStatus, proto.ChangeSet{}, inputsWithAddr)
							if isBlocked {
								fmt.Printf("[%s → %s] skipped (dependency failed)\n", node.ID, target.ID)
							} else {
								fmt.Printf("[%s → %s] skipped (dependency or condition)\n", node.ID, target.ID)
							}
						}
					}

					doneChan <- id
					return
				}

				// Execute on targets
				var nodeErrs []error
				var targetWg sync.WaitGroup
				var targetMu sync.Mutex
				anyChanged := false

				for _, tID := range node.Targets {
					target, ok := targetMap[tID]
					if !ok {
						continue
					}
					targetWg.Add(1)
					sem <- struct{}{}
					go func(t plan.Target) {
						defer func() { <-sem; targetWg.Done() }()
						
						// Check cancel
						select {
						case <-ctx.Done():
							return
						default:
						}

						nodeHash := node.Hash(t.ID)
						latest, err := store.LatestExecution(node.ID, t.ID)
						
						isResumable := false
						isReconciled := false
						
						if err == nil && latest != nil {
							// 1. Resume logic: history-based skip for speed/correctness of interrupted runs
							if opts.Resume && latest.PlanHash == planHash && latest.Status == "applied" {
								isResumable = true
							}

							// 2. Reconcile / Apply logic:
							// v0.8: Both apply and reconcile are now effectively "probed" for file.sync.
							// For process.exec, we still allow history-based skip if not idempotent to avoid unwanted side effects.
							if opts.Reconcile || !opts.Resume {
								if node.Type == "file.sync" {
									isReconciled = false // Always probe (standard apply or reconcile)
								} else if node.Type == "process.exec" {
									if latest.Status == "applied" && latest.NodeHash == nodeHash && !node.Idempotent {
										isReconciled = true
									} else {
										isReconciled = false // Run if idempotent or hash changed
									}
								}
							}
						}

						if isResumable {
							fmt.Printf("[%s → %s] skipped (resumed)\n", node.ID, t.ID)
							targetMu.Lock()
							if latest != nil && (totalChanges(latest.ChangeSet) > 0 || node.Type == "process.exec") {
								anyChanged = true
							}
							targetMu.Unlock()
							return
						}

						if isReconciled {
							fmt.Printf("[%s → %s] ✓ up to date (reconciled from history)\n", node.ID, t.ID)
							targetMu.Lock()
							if latest != nil && (totalChanges(latest.ChangeSet) > 0 || node.Type == "process.exec") {
								anyChanged = true
							}
							targetMu.Unlock()
							return
						}

						// Wrap runNode with retry logic if configured
						var changed bool
						attempts := 1
						if node.Retry != nil && node.Retry.Attempts > 1 {
							attempts = node.Retry.Attempts
						}

						for a := 1; a <= attempts; a++ {
							if a > 1 {
								delay := 5 * time.Second
								if node.Retry.Delay != "" {
									if d, parseErr := time.ParseDuration(node.Retry.Delay); parseErr == nil {
										delay = d
									}
								}
								fmt.Printf("[%s → %s] retrying (%d/%d) after %v...\n", node.ID, t.ID, a, attempts, delay)
								time.Sleep(delay)
							}

							changed, err = runNode(ctx, node, t, planHash, nodeHash, store, opts)
							if err == nil {
								break
							}
							
							// If it's the last attempt, don't retry anymore
							if a == attempts {
								break
							}
						}
						
						targetMu.Lock()
						if err != nil {
							nodeErrs = append(nodeErrs, fmt.Errorf("[%s → %s] %w", node.ID, t.ID, err))
						}
						if changed {
							anyChanged = true
						}
						targetMu.Unlock()
					}(target)
				}
				targetWg.Wait()

				mu.Lock()
				nodeChanged[id] = anyChanged
				if len(nodeErrs) > 0 {
					nodeStates[id] = "failed"
					errs = append(errs, nodeErrs...)
					policy := node.FailurePolicy
					if policy == "" {
						policy = "halt"
					}
					if policy == "halt" || policy == "rollback" {
						haltExecution = true
						cancel() // Stop remaining target executions
					}
					// If "continue", we just don't halt, dependents will cascade skip automatically.
				} else {
					nodeStates[id] = "applied"
				}
				
				isHaltOrRollback := haltExecution && (node.FailurePolicy == "halt" || node.FailurePolicy == "rollback" || node.FailurePolicy == "")
				failedPolicy := node.FailurePolicy
				
				mu.Unlock()

				if len(nodeErrs) > 0 && failedPolicy == "rollback" && isHaltOrRollback {
					fmt.Printf("[%s] triggering global rollback due to failed policy\n", node.ID)
					_ = RollbackLast(store, opts)
				}

				doneChan <- id
			}()
		}
	}()

	// Wait for graph completion
	completed := 0
	for id := range doneChan {
		completed++
		
		mu.Lock()
		// Unlock dependants
		for _, dep := range graph.Edges[id] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				readyQueue <- dep
			}
		}
		mu.Unlock()

		if completed == len(graph.Nodes) {
			break
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

// runNode handles one (node × target) pair. Return (changed, error).
func runNode(ctx context.Context, node plan.Node, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, error) {
	addr := addressWithPort(target.Address)
	if node.Type == "file.sync" {
		return runFileSync(ctx, addr, node, target, planHash, nodeHash, store, opts)
	} else if node.Type == "process.exec" {
		return runProcessExec(ctx, addr, node, target, planHash, nodeHash, store, opts)
	}
	return false, fmt.Errorf("unsupported primitive type: %s", node.Type)
}

func runFileSync(ctx context.Context, addr string, node plan.Node, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, error) {
	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return false, fmt.Errorf("connect to agent %s: %w", addr, err)
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
		return false, fmt.Errorf("detect response: %w", err)
	}
	if detectResp.Error != "" {
		return false, fmt.Errorf("agent detect error: %s", detectResp.Error)
	}
	destTree := detectResp.State

	// ── Step 2: Build source tree (controller-local) ──────────────────────────
	src, _ := node.Inputs["src"].(string)
	srcTree, err := filesync.BuildSourceTree(src)
	if err != nil {
		return false, fmt.Errorf("building source tree: %w", err)
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
		return false, nil
	}

	if opts.DryRun || opts.Observe {
		if opts.DryRun {
			fmt.Printf("[%s → %s] dry-run: %d change(s) would be applied\n",
				node.ID, target.ID, totalChanges(cs))
		} else {
			fmt.Printf("[%s → %s] observe: %d change(s) detected\n",
				node.ID, target.ID, totalChanges(cs))
		}
		return true, nil
	}

	// ── Step 4: Apply ─────────────────────────────────────────────────────────
	conn.Close()
	conn, err = dialAgent(ctx, addr, opts)
	if err != nil {
		return false, fmt.Errorf("re-connect for apply: %w", err)
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
		return false, fmt.Errorf("sending apply_req: %w", err)
	}

	needsTransfer := append(cs.Create, cs.Update...)
	if err := streamFiles(src, needsTransfer, enc); err != nil {
		return false, fmt.Errorf("streaming files: %w", err)
	}

	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return false, fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return false, fmt.Errorf("agent apply error: %s", applyResp.Error)
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
	
	dbStatus := res.Status
	if dbStatus == "success" {
		dbStatus = "applied"
	}
	
	if err := store.Record(node.ID, target.ID, node.Type, planHash, nodeHash, contentHash, dbStatus, cs, inputsWithAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: state record failed: %v\n", err)
	}

	// ── Step 6: Rollback on fatal failure ─────────────────────────────────────
	if res.Status == "failed" {
		if res.RollbackSafe {
			fmt.Printf("[%s → %s] triggering agent-level rollback…\n", node.ID, target.ID)
			if err := doRollback(addr, node, planHash, cs, opts); err != nil {
				fmt.Fprintf(os.Stderr, "WARN: rollback error: %v\n", err)
			}
		}
		return true, fmt.Errorf("apply failed: %s", res.Message)
	}

	return true, nil
}

func runProcessExec(ctx context.Context, addr string, node plan.Node, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, error) {
	if opts.DryRun || opts.Observe {
		cmdArr, _ := node.Inputs["cmd"].([]any)
		if opts.DryRun {
			fmt.Printf("[%s → %s] dry-run: would execute %v\n", node.ID, target.ID, cmdArr)
		} else {
			fmt.Printf("[%s → %s] observe: node execution required (idempotent=%v)\n", node.ID, target.ID, node.Idempotent)
		}
		return true, nil
	}

	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return false, fmt.Errorf("connect to agent %s: %w", addr, err)
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
		return false, fmt.Errorf("sending apply_req: %w", err)
	}

	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return false, fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return false, fmt.Errorf("agent apply error: %s", applyResp.Error)
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
	if len(node.RollbackCmd) > 0 {
		inputsWithAddr["__rollback_cmd"] = node.RollbackCmd
	}
	
	dbStatus := res.Status
	if dbStatus == "success" {
		dbStatus = "applied"
	}
	
	if err := store.Record(node.ID, target.ID, node.Type, planHash, nodeHash, contentHash, dbStatus, proto.ChangeSet{}, inputsWithAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: state record failed: %v\n", err)
	}

	if res.Status == "failed" {
		return true, fmt.Errorf("process failed with exit code %d (class: %s)", res.ExitCode, res.Class)
	}

	return true, nil
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
func doRollback(addr string, node plan.Node, planHash string, cs proto.ChangeSet, opts RunOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return fmt.Errorf("connect for rollback: %w", err)
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
			Primitive:   node.Type,
			PlanHash:    planHash,
			RollbackCmd: node.RollbackCmd,
		},
		Inputs:    node.Inputs,
		ChangeSet: cs,
	}); err != nil {
		return err
	}
	var resp proto.RollbackResp
	if err := readJSON(r, &resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return fmt.Errorf("agent error: %s", resp.Error)
	}
	if resp.Result.Status == "failed" {
		return fmt.Errorf("rollback failed: %s %s", resp.Result.Message, resp.Result.Stderr)
	}
	return nil
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

func RollbackLast(store *state.Store, opts RunOptions) error {
	execs, err := store.LastRun()
	if err != nil {
		return fmt.Errorf("fetch last run: %w", err)
	}
	if len(execs) == 0 {
		return fmt.Errorf("no previous run found to rollback")
	}

	for _, ex := range execs {
		if ex.Status != "applied" && ex.Status != "partial" {
			continue
		}
		// Use stored data to reconstruct a node for rollback
		var rbCmd []string
		if raw, ok := ex.Inputs["__rollback_cmd"]; ok {
			if list, ok := raw.([]any); ok {
				for _, it := range list {
					rbCmd = append(rbCmd, fmt.Sprint(it))
				}
			} else if sl, ok := raw.([]string); ok {
				rbCmd = sl
			}
		}

		node := plan.Node{
			ID:          ex.NodeID,
			Type:        ex.PrimitiveType,
			Inputs:      ex.Inputs,
			RollbackCmd: rbCmd,
		}

		addrStr, _ := ex.Inputs["__target_addr"].(string)
		if addrStr == "" {
			addrStr = ex.Target
		}
		addr := addressWithPort(addrStr)
		if err := doRollback(addr, node, ex.PlanHash, ex.ChangeSet, opts); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: rollback failed for node %s on %s: %v\n", ex.NodeID, ex.Target, err)
		} else {
			fmt.Printf("[%s → %s] successfully rolled back\n", ex.NodeID, ex.Target)
			_ = store.Record(ex.NodeID, ex.Target, ex.PrimitiveType, ex.PlanHash, ex.NodeHash, "rollback_cs", "rolled_back", ex.ChangeSet, ex.Inputs)
		}
	}
	return nil
}

func dialAgent(ctx context.Context, addr string, opts RunOptions) (net.Conn, error) {
	var d net.Dialer
	if opts.TLSCertPath == "" || opts.TLSKeyPath == "" {
		return d.DialContext(ctx, "tcp", addr)
	}

	// Load client cert
	cert, err := tls.LoadX509KeyPair(opts.TLSCertPath, opts.TLSKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Load CA if provided
	if opts.TLSCAPath != "" {
		caCert, err := ioutil.ReadFile(opts.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("read ca cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		config.RootCAs = caCertPool
	}

	tlsDialer := &tls.Dialer{
		NetDialer: &d,
		Config:    config,
	}
	return tlsDialer.DialContext(ctx, "tcp", addr)
}
