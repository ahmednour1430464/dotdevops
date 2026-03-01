package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	agentcontext "devopsctl/internal/agent/context"
	"devopsctl/internal/proto"
)

// handleServiceEnsure manages system services.
func handleServiceEnsure(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	name, _ := inputs["name"].(string)
	if name == "" {
		return proto.Result{Status: "failed", Message: "service.ensure: missing 'name'"}
	}

	state, _ := inputs["state"].(string)
	if state == "" {
		state = "started" // default state
	}

	manager, _ := inputs["manager"].(string)
	if manager == "" {
		// Auto-detect manager
		if _, err := exec.LookPath("systemctl"); err == nil {
			manager = "systemctl"
		} else if _, err := exec.LookPath("service"); err == nil {
			manager = "service"
		} else {
			return proto.Result{Status: "failed", Message: "service.ensure: unable to detect service manager (systemctl or service not found)"}
		}
	}

	var action string
	var args []string

	switch state {
	case "started":
		action = "start"
	case "stopped":
		action = "stop"
	case "restarted":
		action = "restart"
	case "enabled":
		action = "enable"
	case "disabled":
		action = "disable"
	default:
		return proto.Result{Status: "failed", Message: fmt.Sprintf("service.ensure: unknown state %q", state)}
	}

	if manager == "systemctl" {
		args = []string{action, name}
	} else if manager == "service" {
		// service command doesn't support enable/disable properly across all OSes, but we'll try standard chkconfig if needed or just fail
		if action == "enable" || action == "disable" {
			return proto.Result{Status: "failed", Message: "service.ensure: enable/disable not supported with 'service' manager"}
		}
		args = []string{name, action}
	} else {
		return proto.Result{Status: "failed", Message: fmt.Sprintf("service.ensure: unsupported manager %q", manager)}
	}

	cmd := []string{manager}
	cmd = append(cmd, args...)

	execResult, err := executor.ExecuteCommand(context.Background(), cmd, "", 30*time.Second)
	if err != nil {
		errorMsg := ""
		if execResult != nil {
			errorMsg = strings.TrimSpace(execResult.Stderr)
		}
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		return proto.Result{
			Status:  "failed",
			Message: errorMsg,
		}
	}

	return proto.Result{
		Status:  "success",
		Message: fmt.Sprintf("Service %q %s successfully", name, state),
	}
}
