package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"time"

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

	if req.Primitive == "process.exec" {
		res := executeProcessWithContext(ctx, full.Inputs, req.NodeID, auditLogger)
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
func executeProcessWithContext(ctx *agentcontext.ExecutionContext, inputs map[string]any, 
                                nodeID string, auditLogger *agentcontext.AuditLogger) proto.Result {
	executor := &agentcontext.Executor{
		Context:       ctx,
		NodeID:        nodeID,
		PrimitiveType: "process.exec",
		AuditLogger:   auditLogger,
	}

	var cmdArgs []string
	if cmdArrRaw, ok := inputs["cmd"].([]any); ok {
		for _, a := range cmdArrRaw {
			cmdArgs = append(cmdArgs, fmt.Sprint(a))
		}
	} else if cmdArrStr, ok := inputs["cmd"].([]string); ok {
		cmdArgs = cmdArrStr
	}

	if len(cmdArgs) == 0 {
		return proto.Result{
			Status:       "failed",
			RollbackSafe: false,
			Stderr:       "invalid or missing 'cmd' array",
		}
	}

	cwd, _ := inputs["cwd"].(string)
	timeoutSec := float64(0)
	if t, ok := inputs["timeout"].(float64); ok {
		timeoutSec = t
	}

	execCtx := context.Background()
	timeout := time.Duration(timeoutSec * float64(time.Second))

	result, err := executor.ExecuteCommand(execCtx, cmdArgs, cwd, timeout)

	res := proto.Result{
		RollbackSafe: false,
	}

	if result != nil {
		res.ExitCode = result.ExitCode
		res.Stdout = result.Stdout
		res.Stderr = result.Stderr
	}

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			res.Status = "failed"
			res.Class = "execution_error"
		} else {
			res.Status = "failed"
			res.Class = "context_enforcement_error"
			if res.Stderr != "" {
				res.Stderr += "\n"
			}
			res.Stderr += err.Error()
		}
	} else if result != nil && result.ExitCode == 0 {
		res.Status = "success"
	} else {
		res.Status = "failed"
		res.Class = "execution_error"
	}

	return res
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

	if req.Primitive == "process.exec" {
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
