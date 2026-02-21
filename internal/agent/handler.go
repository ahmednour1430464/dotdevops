package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"

	"devopsctl/internal/primitive/filesync"
	"devopsctl/internal/primitive/processexec"
	"devopsctl/internal/proto"
)

// handleConn handles a single controller connection.
// The protocol is line-delimited JSON: one JSON object per line.
// The agent is stateless — each connection is independent.
func handleConn(conn net.Conn) {
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
			handleDetect(line, enc)
		case "apply_req":
			handleApply(line, r, enc)
		case "rollback_req":
			handleRollback(line, enc)
		default:
			log.Printf("[agent] unknown message type: %s", env.Type)
		}
	}
}

// handleDetect walks the destination directory and streams the FileTree back.
func handleDetect(raw []byte, enc *json.Encoder) {
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
func handleApply(raw []byte, r *bufio.Reader, enc *json.Encoder) {
	var full applyReqFull
	if err := json.Unmarshal(raw, &full); err != nil {
		writeError(enc, "apply_resp", "unknown", err)
		return
	}
	req := full.ApplyReq

	if req.Primitive == "process.exec" {
		res := processexec.Apply(full.Inputs)
		_ = enc.Encode(proto.ApplyResp{
			Type:   "apply_resp",
			NodeID: req.NodeID,
			Result: res,
		})
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

	destStr, _ := full.Inputs["dest"].(string)
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

// handleRollback reverts the last apply for the given node.
func handleRollback(raw []byte, enc *json.Encoder) {
	var full rollbackReqFull
	if err := json.Unmarshal(raw, &full); err != nil {
		writeError(enc, "rollback_resp", "unknown", err)
		return
	}
	req := full.RollbackReq

	if req.Primitive == "process.exec" {
		_ = enc.Encode(proto.RollbackResp{
			Type:   "rollback_resp",
			NodeID: req.NodeID,
			Result: proto.Result{Status: "failed", RollbackSafe: false, Message: "process.exec cannot be rolled back"},
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
