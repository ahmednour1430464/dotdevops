package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"

	agentcontext "devopsctl/internal/agent/context"
	"devopsctl/internal/proto"
)

// handleFSWrite performs the atomic _fs.write operation.
func handleFSWrite(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)
	contentRaw, _ := inputs["content"].(string)
	modeStr, _ := inputs["mode"].(string)

	if path == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	mode := os.FileMode(0644)
	if modeStr != "" {
		if m, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
			mode = os.FileMode(m)
		}
	}

	if err := ioutil.WriteFile(path, []byte(contentRaw), mode); err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	return proto.Result{Status: "success", RollbackSafe: true}
}

// handleFSMkdir performs the atomic _fs.mkdir operation.
func handleFSMkdir(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)
	modeStr, _ := inputs["mode"].(string)

	if path == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	mode := os.FileMode(0755)
	if modeStr != "" {
		if m, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
			mode = os.FileMode(m)
		}
	}

	if err := os.MkdirAll(path, mode); err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	return proto.Result{Status: "success", RollbackSafe: true}
}

// handleFSDelete performs the atomic _fs.delete operation.
func handleFSDelete(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)

	if path == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	if err := os.RemoveAll(path); err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	return proto.Result{Status: "success", RollbackSafe: true}
}

// handleFSChmod performs the atomic _fs.chmod operation.
func handleFSChmod(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)
	modeStr, _ := inputs["mode"].(string)

	if path == "" || modeStr == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path' or 'mode'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	m, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return proto.Result{Status: "failed", Stderr: fmt.Sprintf("invalid mode: %v", err)}
	}

	if err := os.Chmod(path, os.FileMode(m)); err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	return proto.Result{Status: "success", RollbackSafe: true}
}

// handleFSChown performs the atomic _fs.chown operation.
func handleFSChown(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)
	uidRaw := inputs["uid"]
	gidRaw := inputs["gid"]

	if path == "" || uidRaw == nil || gidRaw == nil {
		return proto.Result{Status: "failed", Stderr: "missing 'path', 'uid', or 'gid'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	uid, ok1 := uidRaw.(float64)
	gid, ok2 := gidRaw.(float64)
	if !ok1 || !ok2 {
		return proto.Result{Status: "failed", Stderr: "uid and gid must be numbers"}
	}

	if err := os.Chown(path, int(uid), int(gid)); err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	return proto.Result{Status: "success", RollbackSafe: true}
}

// handleExec performs the atomic _exec operation.
func handleExec(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	var cmdArgs []string
	if cmdArrRaw, ok := inputs["cmd"].([]any); ok {
		for _, a := range cmdArrRaw {
			cmdArgs = append(cmdArgs, fmt.Sprint(a))
		}
	}

	if len(cmdArgs) == 0 {
		return proto.Result{Status: "failed", Stderr: "invalid or missing 'cmd' array"}
	}

	cwd, _ := inputs["cwd"].(string)
	timeoutSec := float64(0)
	if t, ok := inputs["timeout"].(float64); ok {
		timeoutSec = t
	}

	execCtx := context.Background()
	timeout := time.Duration(timeoutSec * float64(time.Second))

	// Re-using the robust command execution logic from Executor
	res, err := executor.ExecuteCommand(execCtx, cmdArgs, cwd, timeout)

	protoRes := proto.Result{RollbackSafe: false}
	if res != nil {
		protoRes.ExitCode = res.ExitCode
		protoRes.Stdout = res.Stdout
		protoRes.Stderr = res.Stderr
	}

	if err != nil {
		protoRes.Status = "failed"
		protoRes.Class = "execution_error"
		if protoRes.Stderr != "" {
			protoRes.Stderr += "\n"
		}
		protoRes.Stderr += err.Error()
	} else if res != nil && res.ExitCode == 0 {
		protoRes.Status = "success"
	} else {
		protoRes.Status = "failed"
		protoRes.Class = "execution_error"
	}

	return protoRes
}

// handleFSExists performs the read-only _fs.exists operation.
// Returns whether a file/directory exists at the given path.
func handleFSExists(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)

	if path == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpRead); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	_, err := os.Stat(path)
	exists := err == nil

	// Return result as JSON in Stdout for the controller to parse
	result := map[string]any{
		"exists": exists,
	}
	resultJSON, _ := json.Marshal(result)

	return proto.Result{
		Status:       "success",
		Stdout:       string(resultJSON),
		RollbackSafe: true, // Read-only operation
	}
}

// handleFSStat performs the read-only _fs.stat operation.
// Returns file metadata: mode, uid, gid, size, checksum.
func handleFSStat(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)

	if path == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpRead); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - return empty stat with exists=false
			result := map[string]any{
				"exists": false,
			}
			resultJSON, _ := json.Marshal(result)
			return proto.Result{
				Status:       "success",
				Stdout:       string(resultJSON),
				RollbackSafe: true,
			}
		}
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	// Get uid/gid from syscall stat
	var uid, gid uint32
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = stat.Uid
		gid = stat.Gid
	}

	// Calculate checksum for regular files
	checksum := ""
	if info.Mode().IsRegular() {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()
			h := sha256.New()
			if _, err := io.Copy(h, f); err == nil {
				checksum = hex.EncodeToString(h.Sum(nil))
			}
		}
	}

	result := map[string]any{
		"exists":   true,
		"is_dir":   info.IsDir(),
		"mode":     fmt.Sprintf("%04o", info.Mode().Perm()),
		"uid":      uid,
		"gid":      gid,
		"size":     info.Size(),
		"checksum": checksum,
	}
	resultJSON, _ := json.Marshal(result)

	return proto.Result{
		Status:       "success",
		Stdout:       string(resultJSON),
		RollbackSafe: true, // Read-only operation
	}
}

// handleFSRead performs the read-only _fs.read operation.
// Returns file content as a string (limited to 1MB for safety).
func handleFSRead(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	path, _ := inputs["path"].(string)

	if path == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'path'"}
	}

	if err := executor.ValidateFilePath(path, agentcontext.FileOpRead); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	// Open and read the file with 1MB limit for safety
	f, err := os.Open(path)
	if err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 1<<20)) // 1MB limit
	if err != nil {
		return proto.Result{Status: "failed", Stderr: err.Error()}
	}

	return proto.Result{
		Status:       "success",
		Stdout:       string(data),
		RollbackSafe: true, // Read-only operation
	}
}

// handleNetFetch performs the _net.fetch operation.
// Downloads a URL to a local file on the target.
func handleNetFetch(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	url, _ := inputs["url"].(string)
	dest, _ := inputs["dest"].(string)
	method, _ := inputs["method"].(string)

	if url == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'url'"}
	}
	if dest == "" {
		return proto.Result{Status: "failed", Stderr: "missing 'dest'"}
	}

	// Validate destination path is writable
	if err := executor.ValidateFilePath(dest, agentcontext.FileOpWrite); err != nil {
		return proto.Result{Status: "failed", Class: "context_enforcement_error", Stderr: err.Error()}
	}

	// Default to GET
	if method == "" {
		method = "GET"
	}

	// Create HTTP request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return proto.Result{Status: "failed", Stderr: fmt.Sprintf("create request: %v", err)}
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return proto.Result{Status: "failed", Stderr: fmt.Sprintf("fetch URL: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return proto.Result{Status: "failed", Stderr: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)}
	}

	// Create destination file
	f, err := os.Create(dest)
	if err != nil {
		return proto.Result{Status: "failed", Stderr: fmt.Sprintf("create dest file: %v", err)}
	}
	defer f.Close()

	// Write response body to file (with 100MB limit for safety)
	_, err = io.Copy(f, io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		return proto.Result{Status: "failed", Stderr: fmt.Sprintf("write dest file: %v", err)}
	}

	return proto.Result{
		Status:       "success",
		RollbackSafe: true,
	}
}
