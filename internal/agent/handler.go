package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	agentcontext "devopsctl/internal/agent/context"
	"devopsctl/internal/primitive/filesync"
	"devopsctl/internal/proto"
)

// handleConn handles a single controller connection.
// The protocol is line-delimited JSON: one JSON object per line.
// The agent is stateless — each connection is independent.
func handleConn(conn net.Conn, contexts map[string]*agentcontext.ExecutionContext, auditLogger *agentcontext.AuditLogger) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	enc := json.NewEncoder(conn)

	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[agent] read error: %v", err)
			}
			return
		}

		// Peek at the message type.
		var env proto.Message
		if err := json.Unmarshal(line, &env); err != nil {
			log.Printf("[agent] bad envelope: %v", err)
			continue
		}

		switch env.Type {
		case "detect_req":
			handleDetect(line, enc, contexts)
		case "apply_req":
			handleApply(line, r, enc, contexts, auditLogger)
		case "rollback_req":
			handleRollback(line, enc, contexts, auditLogger)
		case "probe_req":
			handleProbe(line, enc, contexts)
		default:
			log.Printf("[agent] unknown message type: %s", env.Type)
		}
	}
}

// handleDetect walks the destination directory and streams the FileTree back.
func handleDetect(raw []byte, enc *json.Encoder, contexts map[string]*agentcontext.ExecutionContext) {
	var req proto.DetectReq
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(enc, "detect_resp", req.NodeID, err)
		return
	}

	if req.Primitive == "process.exec" {
		_ = enc.Encode(proto.DetectResp{
			Type:   "detect_resp",
			NodeID: req.NodeID,
			State:  proto.FileTree{},
		})
		return
	}

	destRaw := req.Inputs["dest"]
	dest, ok := destRaw.(string)
	if !ok || dest == "" {
		writeError(enc, "detect_resp", req.NodeID, fmt.Errorf("missing string 'dest' in inputs"))
		return
	}
	tree, err := filesync.Detect(dest)
	if err != nil {
		writeError(enc, "detect_resp", req.NodeID, err)
		return
	}
	_ = enc.Encode(proto.DetectResp{
		Type:   "detect_resp",
		NodeID: req.NodeID,
		State:  tree,
	})
}

// handleApply reads the ApplyReq, then reads file chunks from the reader,
// applies them, and sends the result.
func handleApply(raw []byte, r *bufio.Reader, enc *json.Encoder, 
                 contexts map[string]*agentcontext.ExecutionContext, 
                 auditLogger *agentcontext.AuditLogger) {
	var full applyReqFull
	if err := json.Unmarshal(raw, &full); err != nil {
		writeError(enc, "apply_resp", "unknown", err)
		return
	}
	req := full.ApplyReq

	// Resolve execution context for this primitive
	ctx, err := agentcontext.ResolveContext(req.Primitive, contexts)
	if err != nil {
		writeError(enc, "apply_resp", req.NodeID, 
			fmt.Errorf("context resolution: %w", err))
		return
	}

	// Dispatch based on primitive type
	switch req.Primitive {
	case "process.exec":
		res := executeProcessWithContext(ctx, full.Inputs, req.NodeID, auditLogger)
		_ = enc.Encode(proto.ApplyResp{
			Type:   "apply_resp",
			NodeID: req.NodeID,
			Result: res,
		})
		return

	case "_exec":
		executor := &agentcontext.Executor{
			Context:       ctx,
			NodeID:        req.NodeID,
			PrimitiveType: "_exec",
			AuditLogger:   auditLogger,
		}
		res := handleExec(executor, full.Inputs)
		_ = enc.Encode(proto.ApplyResp{
			Type:   "apply_resp",
			NodeID: req.NodeID,
			Result: res,
		})
		return

	case "_fs.write", "_fs.mkdir", "_fs.delete", "_fs.chmod", "_fs.chown", "_fs.exists", "_fs.stat":
		executor := &agentcontext.Executor{
			Context:       ctx,
			NodeID:        req.NodeID,
			PrimitiveType: req.Primitive,
			AuditLogger:   auditLogger,
		}
		var res proto.Result
		switch req.Primitive {
		case "_fs.write":
			res = handleFSWrite(executor, full.Inputs)
		case "_fs.mkdir":
			res = handleFSMkdir(executor, full.Inputs)
		case "_fs.delete":
			res = handleFSDelete(executor, full.Inputs)
		case "_fs.chmod":
			res = handleFSChmod(executor, full.Inputs)
		case "_fs.chown":
			res = handleFSChown(executor, full.Inputs)
		case "_fs.exists":
			res = handleFSExists(executor, full.Inputs)
		case "_fs.stat":
			res = handleFSStat(executor, full.Inputs)
		}
		_ = enc.Encode(proto.ApplyResp{
			Type:   "apply_resp",
			NodeID: req.NodeID,
			Result: res,
		})
		return
	}

	// file.sync with context validation
	destStr, _ := full.Inputs["dest"].(string)

	// Validate file path against context
	executor := &agentcontext.Executor{Context: ctx}
	if err := executor.ValidateFilePath(destStr, agentcontext.FileOpWrite); err != nil {
		writeError(enc, "apply_resp", req.NodeID, err)
		return
	}

	// Build a chunk reader closure that reads from the buffered conn reader.
	chunkReader := func() (*proto.ChunkMsg, error) {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return nil, io.EOF
		}
		var env proto.Message
		if err := json.Unmarshal(line, &env); err != nil || env.Type != "chunk" {
			return nil, io.EOF
		}
		var chunk proto.ChunkMsg
		if err := json.Unmarshal(line, &chunk); err != nil {
			return nil, err
		}
		return &chunk, nil
	}

	result := filesync.Apply(destStr, full.ChangeSet, map[string]string{
		"dest": destStr,
		"mode": fmt.Sprint(full.Inputs["mode"]),
	}, chunkReader) // filesync compatibility wrapper
	
	// Convert filesync Result to standard typed Result (already has Status now)
	result.RollbackSafe = true // File sync allows rollback

	_ = enc.Encode(proto.ApplyResp{
		Type:   "apply_resp",
		NodeID: req.NodeID,
		Result: result,
	})
}

// applyReqFull is an extended ApplyReq that includes inputs for dest, mode, owner/group.
type applyReqFull struct {
	proto.ApplyReq
	Inputs map[string]any `json:"inputs"`
}

// executeProcessWithContext executes a process primitive with context enforcement.
// Legacy wrapper for 'process.exec'.
func executeProcessWithContext(ctx *agentcontext.ExecutionContext, inputs map[string]any, 
                                nodeID string, auditLogger *agentcontext.AuditLogger) proto.Result {
	executor := &agentcontext.Executor{
		Context:       ctx,
		NodeID:        nodeID,
		PrimitiveType: "process.exec",
		AuditLogger:   auditLogger,
	}
	return handleExec(executor, inputs)
}

// handleRollback reverts the last apply for the given node.
func handleRollback(raw []byte, enc *json.Encoder, 
                    contexts map[string]*agentcontext.ExecutionContext, 
                    auditLogger *agentcontext.AuditLogger) {
	var full rollbackReqFull
	if err := json.Unmarshal(raw, &full); err != nil {
		writeError(enc, "rollback_resp", "unknown", err)
		return
	}
	req := full.RollbackReq

	switch req.Primitive {
	case "process.exec":
		if len(req.RollbackCmd) == 0 {
			_ = enc.Encode(proto.RollbackResp{
				Type:   "rollback_resp",
				NodeID: req.NodeID,
				Result: proto.Result{Status: "failed", RollbackSafe: false, Message: "process.exec has no rollback_cmd defined"},
			})
			return
		}

		// Identify execution context
		ctx, err := agentcontext.ResolveContext(req.Primitive, contexts)
		if err != nil {
			writeError(enc, "rollback_resp", req.NodeID, 
				fmt.Errorf("context resolution: %w", err))
			return
		}

		// Wrap RollbackCmd as Inputs for executeProcessWithContext
		rollbackInputs := map[string]any{
			"cmd":     req.RollbackCmd,
			"cwd":     full.Inputs["cwd"],
			"timeout": full.Inputs["timeout"],
		}

		res := executeProcessWithContext(ctx, rollbackInputs, req.NodeID, auditLogger)
		_ = enc.Encode(proto.RollbackResp{
			Type:   "rollback_resp",
			NodeID: req.NodeID,
			Result: res,
		})
		return

	case "_exec":
		if len(req.RollbackCmd) == 0 {
			_ = enc.Encode(proto.RollbackResp{
				Type:   "rollback_resp",
				NodeID: req.NodeID,
				Result: proto.Result{Status: "failed", RollbackSafe: false, Message: "_exec has no rollback_cmd for this node"},
			})
			return
		}
		ctx, err := agentcontext.ResolveContext(req.Primitive, contexts)
		if err != nil {
			writeError(enc, "rollback_resp", req.NodeID, fmt.Errorf("context resolution: %w", err))
			return
		}
		executor := &agentcontext.Executor{
			Context:       ctx,
			NodeID:        req.NodeID,
			PrimitiveType: "_exec (rollback)",
			AuditLogger:   auditLogger,
		}
		rollbackInputs := map[string]any{
			"cmd":     req.RollbackCmd,
			"cwd":     full.Inputs["cwd"],
			"timeout": full.Inputs["timeout"],
		}
		res := handleExec(executor, rollbackInputs)
		_ = enc.Encode(proto.RollbackResp{
			Type:   "rollback_resp",
			NodeID: req.NodeID,
			Result: res,
		})
		return
	}

	destStr, _ := full.Inputs["dest"].(string)
	result := filesync.Rollback(destStr, full.ChangeSet)

	_ = enc.Encode(proto.RollbackResp{
		Type:   "rollback_resp",
		NodeID: req.NodeID,
		Result: result,
	})
}

type rollbackReqFull struct {
	proto.RollbackReq
	Inputs    map[string]any  `json:"inputs"`
	ChangeSet proto.ChangeSet `json:"changeset"`
}

func writeError(enc *json.Encoder, msgType, nodeID string, err error) {
	log.Printf("[agent] %s error (node=%s): %v", msgType, nodeID, err)
	_ = enc.Encode(map[string]string{
		"type":    msgType,
		"node_id": nodeID,
		"error":   err.Error(),
	})
}

// handleProbe evaluates probe expressions and returns observed state (v1.3+).
func handleProbe(raw []byte, enc *json.Encoder, contexts map[string]*agentcontext.ExecutionContext) {
	var req proto.ProbeReq
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(enc, "probe_resp", "unknown", err)
		return
	}

	// Resolve execution context
	ctx, err := agentcontext.ResolveContext(req.Primitive, contexts)
	if err != nil {
		writeError(enc, "probe_resp", req.NodeID, fmt.Errorf("context resolution: %w", err))
		return
	}

	executor := &agentcontext.Executor{
		Context:       ctx,
		NodeID:        req.NodeID,
		PrimitiveType: req.Primitive + " (probe)",
	}

	// Evaluate each probe field
	state := make(map[string]any)
	for fieldName, exprData := range req.Probe {
		result, err := evaluateProbeExpr(executor, exprData)
		if err != nil {
			log.Printf("[agent] probe field %q error: %v", fieldName, err)
			state[fieldName] = nil // Mark as error/unknown
		} else {
			state[fieldName] = result
		}
	}

	_ = enc.Encode(proto.ProbeResp{
		Type:   "probe_resp",
		NodeID: req.NodeID,
		State:  state,
	})
}

// evaluateProbeExpr evaluates a serialized probe expression.
// The expression is a map like {"func": "_fs.exists", "args": [{"literal": "/path"}]}
func evaluateProbeExpr(executor *agentcontext.Executor, exprData any) (any, error) {
	exprMap, ok := exprData.(map[string]any)
	if !ok {
		// It's a literal value, return as-is
		return exprData, nil
	}

	// Check if it's a function call
	funcName, hasFunc := exprMap["func"].(string)
	if !hasFunc {
		// It's a plain value or unknown format
		return exprData, nil
	}

	// Get arguments
	argsRaw, _ := exprMap["args"].([]any)
	var args []any
	for _, argRaw := range argsRaw {
		// Recursively evaluate arguments (they might be nested calls)
		eval, err := evaluateProbeExpr(executor, argRaw)
		if err != nil {
			return nil, err
		}
		args = append(args, eval)
	}

	// Dispatch to appropriate probe function
	switch funcName {
	case "_fs.exists":
		if len(args) < 1 {
			return nil, fmt.Errorf("_fs.exists requires 1 argument")
		}
		path, _ := args[0].(string)
		result := handleFSExists(executor, map[string]any{"path": path})
		if result.Status == "success" {
			// Parse JSON result from stdout
			var out map[string]any
			json.Unmarshal([]byte(result.Stdout), &out)
			return out["exists"], nil
		}
		return nil, fmt.Errorf(result.Stderr)

	case "_fs.stat":
		if len(args) < 1 {
			return nil, fmt.Errorf("_fs.stat requires 1 argument")
		}
		path, _ := args[0].(string)
		result := handleFSStat(executor, map[string]any{"path": path})
		if result.Status == "success" {
			// Parse JSON result from stdout and return the full stat object
			var out map[string]any
			json.Unmarshal([]byte(result.Stdout), &out)
			return out, nil
		}
		return nil, fmt.Errorf(result.Stderr)

	case "_fs.read":
		if len(args) < 1 {
			return nil, fmt.Errorf("_fs.read requires 1 argument")
		}
		path, _ := args[0].(string)
		if err := executor.ValidateFilePath(path, agentcontext.FileOpRead); err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(mustOpen(path), 1<<20)) // 1MB limit
		if err != nil {
			return nil, err
		}
		return string(data), nil

	default:
		return nil, fmt.Errorf("unknown probe function: %s", funcName)
	}
}

// mustOpen is a helper for probe evaluation.
func mustOpen(path string) io.Reader {
	f, err := os.Open(path)
	if err != nil {
		return strings.NewReader("")
	}
	return f
}
