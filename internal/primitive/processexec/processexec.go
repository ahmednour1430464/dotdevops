package processexec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"devopsctl/internal/proto"
)

// Apply executes a local process based on inputs.
func Apply(inputs map[string]any) proto.Result {
	cmdArrRaw, ok := inputs["cmd"].([]any)
	if !ok || len(cmdArrRaw) == 0 {
		return proto.Result{Status: "failed", RollbackSafe: false, Stderr: "invalid or missing 'cmd' array"}
	}
	cwd, _ := inputs["cwd"].(string)

	var cmdArgs []string
	for _, a := range cmdArrRaw {
		cmdArgs = append(cmdArgs, fmt.Sprint(a))
	}

	timeoutSec := float64(0)
	if t, ok := inputs["timeout"].(float64); ok {
		timeoutSec = t
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec*float64(time.Second)))
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	
	res := proto.Result{
		Status:       "success",
		RollbackSafe: false,
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
	}

	if err != nil {
		res.Status = "failed"
		res.Class = "execution_error"
		
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
			if res.Stderr != "" {
				res.Stderr += "\n"
			}
			res.Stderr += err.Error()
		}
		
		if ctx.Err() == context.DeadlineExceeded {
			res.Class = "timeout"
			if res.Stderr != "" {
				res.Stderr += "\n"
			}
			res.Stderr += "process timed out"
		}
	} else {
		res.ExitCode = 0
	}

	return res
}
