// Package controller implements the orchestrator that connects to agents,
// runs the detect-diff-apply cycle, and manages state.
package controller

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"
	"time"

	"devopsctl/internal/plan"
	"devopsctl/internal/primitive/filesync"
	"devopsctl/internal/proto"
	"devopsctl/internal/secret"
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

	// SecretProvider resolves secret() references in the plan at apply-time.
	// If nil, secrets are resolved from environment variables (EnvProvider).
	SecretProvider secret.Provider

	Confirm bool // Automatically confirm dangerous rollback skips

	OutputFormat string // "text" or "json"
}

// ExecutionResult represents the outcome of a single node/target execution.
type ExecutionResult struct {
	Node       string `json:"node"`
	Target     string `json:"target"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// ObservationResult represents the outcome of a single node/target observation.
type ObservationResult struct {
	Node     string `json:"node"`
	Target   string `json:"target"`
	Desired  any    `json:"desired,omitempty"`
	Observed any    `json:"observed,omitempty"`
	InSync   bool   `json:"in_sync"`
}

// RollbackResult represents the summary of a rollback run.
type RollbackResult struct {
	RolledBack []string `json:"rolled_back"`
	Skipped    []string `json:"skipped"`
	Errors     []string `json:"errors,omitempty"`
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

	var results []any
	var resultsMu sync.Mutex

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
							inputsWithAddr = secret.RedactNodeInputs(inputsWithAddr, node.RequiresSecrets)
							nodeHash := node.Hash(target.ID)
							_ = store.Record(node.ID, target.ID, node.Type, planHash, nodeHash, "skipped_cs", recStatus, proto.ChangeSet{}, inputsWithAddr)
							if opts.OutputFormat == "json" {
								resultsMu.Lock()
								results = append(results, ExecutionResult{
									Node:   node.ID,
									Target: target.ID,
									Status: "skipped",
								})
								resultsMu.Unlock()
							} else {
								if isBlocked {
									fmt.Printf("[%s → %s] skipped (dependency failed)\n", node.ID, target.ID)
								} else {
									fmt.Printf("[%s → %s] skipped (dependency or condition)\n", node.ID, target.ID)
								}
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
							if opts.OutputFormat == "json" {
								resultsMu.Lock()
								results = append(results, ExecutionResult{
									Node:   node.ID,
									Target: t.ID,
									Status: "skipped",
								})
								resultsMu.Unlock()
							} else {
								fmt.Printf("[%s → %s] skipped (resumed)\n", node.ID, t.ID)
							}
							targetMu.Lock()
							if latest != nil && (totalChanges(latest.ChangeSet) > 0 || node.Type == "process.exec") {
								anyChanged = true
							}
							targetMu.Unlock()
							return
						}

						if isReconciled {
							if opts.OutputFormat == "json" {
								resultsMu.Lock()
								results = append(results, ExecutionResult{
									Node:   node.ID,
									Target: t.ID,
									Status: "up-to-date",
								})
								resultsMu.Unlock()
							} else {
								fmt.Printf("[%s → %s] ✓ up to date (reconciled from history)\n", node.ID, t.ID)
							}
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
								if opts.OutputFormat != "json" {
									fmt.Printf("[%s → %s] retrying (%d/%d) after %v...\n", node.ID, t.ID, a, attempts, delay)
								}
								time.Sleep(delay)
							}

							start := time.Now()
							var res any
							changed, res, err = runNode(ctx, node, t, planHash, nodeHash, store, opts)
							duration := time.Since(start)

							if opts.OutputFormat == "json" {
								if res != nil {
									if er, ok := res.(ExecutionResult); ok {
										er.DurationMS = duration.Milliseconds()
										resultsMu.Lock()
										results = append(results, er)
										resultsMu.Unlock()
									} else {
										resultsMu.Lock()
										results = append(results, res)
										resultsMu.Unlock()
									}
								} else if err != nil {
									resultsMu.Lock()
									results = append(results, ExecutionResult{
										Node:       node.ID,
										Target:     t.ID,
										Status:     "failed",
										DurationMS: duration.Milliseconds(),
										Error:      err.Error(),
									})
									resultsMu.Unlock()
								}
							}

							if err == nil {
								break
							}

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
		if opts.OutputFormat != "json" {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", e)
			}
		}
		if opts.OutputFormat == "json" {
			b, _ := json.MarshalIndent(results, "", "  ")
			fmt.Println(string(b))
		}
		return fmt.Errorf("%d node(s) failed", len(errs))
	}
	if opts.OutputFormat == "json" {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
	}
	return nil
}

// runNode handles one (node × target) pair. Return (changed, result, error).
func runNode(ctx context.Context, node plan.Node, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, any, error) {
	// ── Resolve secrets before dispatching to agent ───────────────────────────
	resolvedInputs := node.Inputs
	if len(node.RequiresSecrets) > 0 {
		provider := opts.SecretProvider
		if provider == nil {
			provider = &secret.EnvProvider{}
		}
		var err error
		resolvedInputs, err = secret.ResolveNodeInputs(node.Inputs, provider)
		if err != nil {
			return false, nil, fmt.Errorf("secret resolution via %s: %w", provider.Name(), err)
		}
	}

	addr := addressWithPort(target.Address)

	// ── v1.3+ Probe-based state comparison ─────────────────────────────────────
	if node.Probe != nil && node.Desired != nil {
		observed, err := runProbe(ctx, addr, node, opts)
		if err != nil {
			// Probe failed - continue to run body (conservative)
			if opts.OutputFormat != "json" {
				fmt.Fprintf(os.Stderr, "WARN: probe failed for %s: %v\n", node.ID, err)
			}
		} else {
			// Compare probe result with desired state
			matches, diffs := compareState(observed, node.Desired)

			if opts.Observe {
				if matches {
					if opts.OutputFormat != "json" {
						fmt.Printf("node %q → %s:\n  [OK] all fields match desired state\n", node.ID, target.ID)
					}
					return false, ObservationResult{
						Node:     node.ID,
						Target:   target.ID,
						Desired:  node.Desired,
						Observed: observed,
						InSync:   true,
					}, nil
				}
				if opts.OutputFormat != "json" {
					printStateDiff(node.ID, target.ID, diffs)
				}
				return true, ObservationResult{
					Node:     node.ID,
					Target:   target.ID,
					Desired:  node.Desired,
					Observed: observed,
					InSync:   false,
				}, nil
			}

			if matches {
				// Node already in desired state - skip execution
				if opts.OutputFormat != "json" {
					fmt.Printf("[%s → %s] ✓ already satisfied (probe matches desired)\n", node.ID, target.ID)
				}
				return false, ExecutionResult{
					Node:   node.ID,
					Target: target.ID,
					Status: "satisfied",
				}, nil
			}

			// State differs - show diff and proceed to apply
			if opts.OutputFormat != "json" {
				for _, d := range diffs {
					desStr := formatValue(d.Desired)
					obsStr := formatValue(d.Observed)
					fmt.Printf("[%s → %s] %s: %s → %s\n", node.ID, target.ID, d.Field, obsStr, desStr)
				}
			}
		}
	}

	if node.Type == "file.sync" {
		return runFileSync(ctx, addr, node, resolvedInputs, target, planHash, nodeHash, store, opts)
	} else if node.Type == "process.exec" {
		return runProcessExec(ctx, addr, node, resolvedInputs, target, planHash, nodeHash, store, opts)
	} else if isBuiltin(node.Type) {
		return runBuiltin(ctx, addr, node, resolvedInputs, target, planHash, nodeHash, store, opts)
	}
	return false, nil, fmt.Errorf("unsupported primitive type: %s", node.Type)
}

func isBuiltin(t string) bool {
	switch t {
	case "_fs.write", "_fs.read", "_fs.mkdir", "_fs.delete", "_fs.chmod", "_fs.chown", "_fs.exists", "_fs.stat", "_net.fetch", "_exec":
		return true
	}
	return false
}

func runBuiltin(ctx context.Context, addr string, node plan.Node, resolvedInputs map[string]any, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, any, error) {
	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return false, nil, fmt.Errorf("connect to agent %s: %w", addr, err)
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
		Inputs: resolvedInputs,
	}
	if err := enc.Encode(applyReq); err != nil {
		return false, nil, fmt.Errorf("sending apply_req: %w", err)
	}

	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return false, nil, fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return false, nil, fmt.Errorf("agent apply error: %s", applyResp.Error)
	}

	res := applyResp.Result
	if opts.OutputFormat != "json" {
		fmt.Printf("[%s → %s] builtin %s: %s\n", node.ID, target.ID, node.Type, res.Status)
		if res.Stdout != "" {
			fmt.Printf("--- stdout ---\n%s\n--------------\n", res.Stdout)
		}
		if res.Stderr != "" {
			fmt.Printf("--- stderr ---\n%s\n--------------\n", res.Stderr)
		}
	}

	contentHash := "builtin_no_cs"
	inputsWithAddr := make(map[string]any)
	for k, v := range node.Inputs {
		inputsWithAddr[k] = v
	}
	inputsWithAddr["__target_addr"] = target.Address
	inputsWithAddr = secret.RedactNodeInputs(inputsWithAddr, node.RequiresSecrets)

	dbStatus := res.Status
	if dbStatus == "success" {
		dbStatus = "applied"
	}

	if err := store.Record(node.ID, target.ID, node.Type, planHash, nodeHash, contentHash, dbStatus, proto.ChangeSet{}, inputsWithAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: state record failed: %v\n", err)
	}

	if res.Status == "failed" {
		return true, ExecutionResult{
			Node:   node.ID,
			Target: target.ID,
			Status: "failed",
			Error:  res.Message,
		}, fmt.Errorf("builtin failed: %s", res.Message)
	}

	return true, ExecutionResult{
		Node:   node.ID,
		Target: target.ID,
		Status: "applied",
	}, nil
}

func runFileSync(ctx context.Context, addr string, node plan.Node, resolvedInputs map[string]any, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, any, error) {
	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return false, nil, fmt.Errorf("connect to agent %s: %w", addr, err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)

	// ── Step 1: Detect remote state ──────────────────────────────────────────
	_ = enc.Encode(proto.DetectReq{
		Type:      "detect_req",
		NodeID:    node.ID,
		Primitive: node.Type,
		Inputs:    resolvedInputs,
	})
	var detectResp proto.DetectResp
	if err := readJSON(r, &detectResp); err != nil {
		return false, nil, fmt.Errorf("detect response: %w", err)
	}
	if detectResp.Error != "" {
		return false, nil, fmt.Errorf("agent detect error: %s", detectResp.Error)
	}
	destTree := detectResp.State

	// ── Step 2: Build source tree (controller-local) ──────────────────────────
	src, _ := resolvedInputs["src"].(string)
	srcTree, err := filesync.BuildSourceTree(src)
	if err != nil {
		return false, nil, fmt.Errorf("building source tree: %w", err)
	}

	var deleteExtra bool
	switch v := resolvedInputs["delete_extra"].(type) {
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
		if opts.OutputFormat != "json" {
			fmt.Printf("[%s → %s] ✓ no changes\n", node.ID, target.ID)
		}
		if opts.Observe {
			return false, ObservationResult{Node: node.ID, Target: target.ID, InSync: true}, nil
		}
		return false, ExecutionResult{Node: node.ID, Target: target.ID, Status: "applied"}, nil
	}

	if opts.DryRun || opts.Observe {
		if opts.OutputFormat != "json" {
			if opts.DryRun {
				fmt.Printf("[%s → %s] dry-run: %d change(s) would be applied\n",
					node.ID, target.ID, totalChanges(cs))
			} else {
				fmt.Printf("[%s → %s] observe: %d change(s) detected\n",
					node.ID, target.ID, totalChanges(cs))
			}
		}
		if opts.Observe {
			return true, ObservationResult{
				Node:     node.ID,
				Target:   target.ID,
				Desired:  srcTree, // simplified for now
				Observed: destTree,
				InSync:   false,
			}, nil
		}
		return true, ExecutionResult{Node: node.ID, Target: target.ID, Status: "dry-run"}, nil
	}

	// ── Step 4: Apply ─────────────────────────────────────────────────────────
	conn.Close()
	conn, err = dialAgent(ctx, addr, opts)
	if err != nil {
		return false, nil, fmt.Errorf("re-connect for apply: %w", err)
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
		Inputs: resolvedInputs,
	}
	if err := enc.Encode(applyReq); err != nil {
		return false, nil, fmt.Errorf("sending apply_req: %w", err)
	}

	needsTransfer := append(cs.Create, cs.Update...)
	if err := streamFiles(src, needsTransfer, enc); err != nil {
		return false, nil, fmt.Errorf("streaming files: %w", err)
	}

	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return false, nil, fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return false, nil, fmt.Errorf("agent apply error: %s", applyResp.Error)
	}

	res := applyResp.Result
	if opts.OutputFormat != "json" {
		fmt.Printf("[%s → %s] %s\n", node.ID, target.ID, res.Message)
	}

	// ── Step 5: Persist state ──────────────────────────────────────────────────
	contentHash := hashChangeSet(cs)
	inputsWithAddr := make(map[string]any)
	for k, v := range node.Inputs {
		inputsWithAddr[k] = v
	}
	inputsWithAddr["__target_addr"] = target.Address
	if node.SideEffects != "" {
		inputsWithAddr["__side_effects"] = node.SideEffects
	}
	inputsWithAddr = secret.RedactNodeInputs(inputsWithAddr, node.RequiresSecrets)

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
				if opts.OutputFormat != "json" {
					fmt.Fprintf(os.Stderr, "WARN: rollback error: %v\n", err)
				}
			}
		}
		return true, ExecutionResult{
			Node:   node.ID,
			Target: target.ID,
			Status: "failed",
			Error:  res.Message,
		}, fmt.Errorf("apply failed: %s", res.Message)
	}

	return true, ExecutionResult{
		Node:   node.ID,
		Target: target.ID,
		Status: "applied",
	}, nil
}

func runProcessExec(ctx context.Context, addr string, node plan.Node, resolvedInputs map[string]any, target plan.Target, planHash, nodeHash string, store *state.Store, opts RunOptions) (bool, any, error) {
	if opts.DryRun || opts.Observe {
		cmdArr, _ := resolvedInputs["cmd"].([]any)
		if opts.OutputFormat != "json" {
			if opts.DryRun {
				fmt.Printf("[%s → %s] dry-run: would execute %v\n", node.ID, target.ID, cmdArr)
			} else {
				fmt.Printf("[%s → %s] observe: node execution required (idempotent=%v)\n", node.ID, target.ID, node.Idempotent)
			}
		}
		if opts.Observe {
			return true, ObservationResult{
				Node:     node.ID,
				Target:   target.ID,
				Desired:  cmdArr,
				Observed: "not executed",
				InSync:   false,
			}, nil
		}
		return true, ExecutionResult{Node: node.ID, Target: target.ID, Status: "dry-run"}, nil
	}

	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return false, nil, fmt.Errorf("connect to agent %s: %w", addr, err)
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
		Inputs: resolvedInputs,
	}
	if err := enc.Encode(applyReq); err != nil {
		return false, nil, fmt.Errorf("sending apply_req: %w", err)
	}

	var applyResp proto.ApplyResp
	if err := readJSON(r, &applyResp); err != nil {
		return false, nil, fmt.Errorf("apply response: %w", err)
	}
	if applyResp.Error != "" {
		return false, nil, fmt.Errorf("agent apply error: %s", applyResp.Error)
	}

	res := applyResp.Result
	if opts.OutputFormat != "json" {
		fmt.Printf("[%s → %s] process exited with code %d\n", node.ID, target.ID, res.ExitCode)
		if res.Stdout != "" {
			fmt.Printf("--- stdout ---\n%s\n--------------\n", res.Stdout)
		}
		if res.Stderr != "" {
			fmt.Printf("--- stderr ---\n%s\n--------------\n", res.Stderr)
		}
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
	if node.SideEffects != "" {
		inputsWithAddr["__side_effects"] = node.SideEffects
	}
	inputsWithAddr = secret.RedactNodeInputs(inputsWithAddr, node.RequiresSecrets)

	dbStatus := res.Status
	if dbStatus == "success" {
		dbStatus = "applied"
	}

	if err := store.Record(node.ID, target.ID, node.Type, planHash, nodeHash, contentHash, dbStatus, proto.ChangeSet{}, inputsWithAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: state record failed: %v\n", err)
	}

	if res.Status == "failed" {
		return true, ExecutionResult{
			Node:   node.ID,
			Target: target.ID,
			Status: "failed",
			Error:  fmt.Sprintf("exit code %d: %s", res.ExitCode, res.Message),
		}, fmt.Errorf("process failed with exit code %d (class: %s)", res.ExitCode, res.Class)
	}

	return true, ExecutionResult{
		Node:   node.ID,
		Target: target.ID,
		Status: "applied",
	}, nil
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
			Type:        "rollback_req",
			NodeID:      node.ID,
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
		if opts.OutputFormat == "json" {
			b, _ := json.Marshal(RollbackResult{Errors: []string{"no previous run found to rollback"}})
			fmt.Println(string(b))
		}
		return fmt.Errorf("no previous run found to rollback")
	}

	var res RollbackResult
	var w *tabwriter.Writer

	if opts.OutputFormat != "json" {
		fmt.Println("ROLLBACK PLAN")
		fmt.Println("-------------")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NODE\tTARGET\tTYPE\tROLLBACK\tREASON")
	}

	needsConfirm := false
	var validExecs []state.Execution
	for _, ex := range execs {
		if ex.Status != "applied" && ex.Status != "partial" {
			continue
		}
		validExecs = append(validExecs, ex)

		canRollback := "YES"
		reason := ""
		if ex.PrimitiveType == "file.sync" {
			reason = "restoring snapshot"
		} else if ex.PrimitiveType == "process.exec" {
			if _, ok := ex.Inputs["__rollback_cmd"]; ok {
				reason = "using rollback_cmd"
			} else {
				se, _ := ex.Inputs["__side_effects"].(string)
				if se == "" {
					se = "target"
				}
				if se != "none" {
					canRollback = "NO"
					reason = fmt.Sprintf("no rollback_cmd; side_effects=%s", se)
					needsConfirm = true
				} else {
					canRollback = "SKIP"
					reason = "side_effects=none"
				}
			}
		}
		if opts.OutputFormat != "json" {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ex.NodeID, ex.Target, ex.PrimitiveType, canRollback, reason)
		}
	}

	if opts.OutputFormat != "json" {
		w.Flush()
		fmt.Println()
	}

	if needsConfirm && !opts.Confirm {
		return fmt.Errorf("rollback plan contains irreversible nodes; use --confirm to skip them and proceed")
	}

	for _, ex := range validExecs {
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

		if ex.PrimitiveType == "process.exec" && len(rbCmd) == 0 {
			se, _ := ex.Inputs["__side_effects"].(string)
			if se == "" {
				se = "target"
			}
			if se == "none" {
				if opts.OutputFormat != "json" {
					fmt.Printf("[%s → %s] skipped rollback (no side effects)\n", ex.NodeID, ex.Target)
				}
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s on %s", ex.NodeID, ex.Target))
			} else {
				if opts.OutputFormat != "json" {
					fmt.Printf("[%s → %s] skipped rollback (irreversible)\n", ex.NodeID, ex.Target)
				}
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s on %s (irreversible)", ex.NodeID, ex.Target))
			}
			// Record skipped rollback
			_ = store.Record(ex.NodeID, ex.Target, ex.PrimitiveType, ex.PlanHash, ex.NodeHash, "rollback_cs", "skipped", ex.ChangeSet, ex.Inputs)
			continue
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
			if opts.OutputFormat != "json" {
				fmt.Fprintf(os.Stderr, "WARN: rollback failed for node %s on %s: %v\n", ex.NodeID, ex.Target, err)
			}
			res.Errors = append(res.Errors, fmt.Sprintf("%s on %s: %v", ex.NodeID, ex.Target, err))
		} else {
			if opts.OutputFormat != "json" {
				fmt.Printf("[%s → %s] successfully rolled back\n", ex.NodeID, ex.Target)
			}
			res.RolledBack = append(res.RolledBack, fmt.Sprintf("%s on %s", ex.NodeID, ex.Target))
			_ = store.Record(ex.NodeID, ex.Target, ex.PrimitiveType, ex.PlanHash, ex.NodeHash, "rollback_cs", "rolled_back", ex.ChangeSet, ex.Inputs)
		}
	}

	if opts.OutputFormat == "json" {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
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

// runProbe sends a probe request to the agent and returns the observed state (v1.3+).
func runProbe(ctx context.Context, addr string, node plan.Node, opts RunOptions) (map[string]any, error) {
	if node.Probe == nil {
		return nil, nil // No probe defined
	}

	conn, err := dialAgent(ctx, addr, opts)
	if err != nil {
		return nil, fmt.Errorf("connect for probe: %w", err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)

	probeReq := proto.ProbeReq{
		Type:      "probe_req",
		NodeID:    node.ID,
		Primitive: node.Type,
		Probe:     node.Probe,
	}
	if err := enc.Encode(probeReq); err != nil {
		return nil, fmt.Errorf("sending probe_req: %w", err)
	}

	var probeResp proto.ProbeResp
	if err := readJSON(r, &probeResp); err != nil {
		return nil, fmt.Errorf("probe response: %w", err)
	}
	if probeResp.Error != "" {
		return nil, fmt.Errorf("agent probe error: %s", probeResp.Error)
	}

	return probeResp.State, nil
}

// StateDiff represents a single field difference between observed and desired state.
type StateDiff struct {
	Field    string `json:"field"`
	Observed any    `json:"observed"`
	Desired  any    `json:"desired"`
}

// compareState compares observed state from probe with desired state.
// Returns true if they match, along with a list of differences.
func compareState(observed, desired map[string]any) (bool, []StateDiff) {
	var diffs []StateDiff

	for field, desiredVal := range desired {
		observedVal, exists := observed[field]
		if !exists {
			// Field missing in observed state
			diffs = append(diffs, StateDiff{
				Field:    field,
				Observed: nil,
				Desired:  desiredVal,
			})
			continue
		}

		if !deepEqual(observedVal, desiredVal) {
			diffs = append(diffs, StateDiff{
				Field:    field,
				Observed: observedVal,
				Desired:  desiredVal,
			})
		}
	}

	return len(diffs) == 0, diffs
}

// deepEqual compares two values for equality, handling maps and slices.
func deepEqual(a, b any) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare maps
	aMap, aIsMap := a.(map[string]any)
	bMap, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		if len(aMap) != len(bMap) {
			return false
		}
		for k, v := range aMap {
			if !deepEqual(v, bMap[k]) {
				return false
			}
		}
		return true
	}

	// Compare slices
	aSlice, aIsSlice := a.([]any)
	bSlice, bIsSlice := b.([]any)
	if aIsSlice && bIsSlice {
		if len(aSlice) != len(bSlice) {
			return false
		}
		for i := range aSlice {
			if !deepEqual(aSlice[i], bSlice[i]) {
				return false
			}
		}
		return true
	}

	// Handle numeric comparisons (JSON numbers can be float64 or int)
	aFloat, aIsFloat := a.(float64)
	bFloat, bIsFloat := b.(float64)
	aInt, aIsInt := a.(int)
	bInt, bIsInt := b.(int)

	if aIsFloat && bIsInt {
		return aFloat == float64(bInt)
	}
	if aIsInt && bIsFloat {
		return float64(aInt) == bFloat
	}

	// Direct comparison for strings, bools, and identical types
	return a == b
}

// printStateDiff prints state differences in a human-readable format.
func printStateDiff(nodeID, targetID string, diffs []StateDiff) {
	fmt.Printf("node %q → %s:\n", nodeID, targetID)
	for _, d := range diffs {
		desStr := formatValue(d.Desired)
		obsStr := formatValue(d.Observed)
		if d.Observed == nil {
			fmt.Printf("  %s: observed=<missing>  desired=%s  [MISMATCH]\n", d.Field, desStr)
		} else {
			fmt.Printf("  %s: observed=%s  desired=%s  [MISMATCH]\n", d.Field, obsStr, desStr)
		}
	}
}

// formatValue formats a value for display in diff output.
func formatValue(v any) string {
	if v == nil {
		return "<nil>"
	}
	switch val := v.(type) {
	case string:
		if len(val) > 50 {
			return fmt.Sprintf("%q...", val[:50])
		}
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case int, int64, float64:
		return fmt.Sprintf("%v", val)
	default:
		b, _ := json.Marshal(v)
		if len(b) > 50 {
			return string(b[:50]) + "..."
		}
		return string(b)
	}
}
