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

// handlePackageInstall manages system packages.
func handlePackageInstall(executor *agentcontext.Executor, inputs map[string]any) proto.Result {
	// Parse names (can be string or []any of strings)
	var names []string
	if nameRaw, ok := inputs["name"]; ok {
		if s, isStr := nameRaw.(string); isStr {
			names = append(names, s)
		} else if arr, isArr := nameRaw.([]any); isArr {
			for _, item := range arr {
				if s, isStr := item.(string); isStr {
					names = append(names, s)
				}
			}
		}
	}

	if len(names) == 0 {
		return proto.Result{Status: "failed", Message: "package.install: missing or invalid 'name' list"}
	}

	state, _ := inputs["state"].(string)
	if state == "" {
		state = "present"
	}

	updateCache, _ := inputs["update_cache"].(bool)

	manager, _ := inputs["manager"].(string)
	if manager == "" {
		// Auto-detect manager
		if _, err := exec.LookPath("apt-get"); err == nil {
			manager = "apt"
		} else if _, err := exec.LookPath("yum"); err == nil {
			manager = "yum"
		} else if _, err := exec.LookPath("dnf"); err == nil {
			manager = "dnf"
		} else if _, err := exec.LookPath("apk"); err == nil {
			manager = "apk"
		} else if _, err := exec.LookPath("brew"); err == nil {
			manager = "brew"
		} else {
			return proto.Result{Status: "failed", Message: "package.install: unable to detect package manager"}
		}
	}

	// Maybe update cache first
	if updateCache {
		var updateCmd []string
		switch manager {
		case "apt":
			updateCmd = []string{"apt-get", "update", "-y"}
		case "yum", "dnf":
			updateCmd = []string{manager, "makecache"}
		case "apk":
			updateCmd = []string{"apk", "update"}
		case "brew":
			updateCmd = []string{"brew", "update"}
		}
		
		if len(updateCmd) > 0 {
			_, err := executor.ExecuteCommand(context.Background(), updateCmd, "", 2*time.Minute)
			if err != nil {
				return proto.Result{Status: "failed", Message: fmt.Sprintf("package.install: failed to update cache: %v", err)}
			}
		}
	}

	var installCmd []string
	switch manager {
	case "apt":
		installCmd = []string{"apt-get"}
		if state == "absent" {
			installCmd = append(installCmd, "remove", "-y")
		} else {
			// "present" or "latest"
			installCmd = append(installCmd, "install", "-y")
		}
	case "yum", "dnf":
		installCmd = []string{manager}
		if state == "absent" {
			installCmd = append(installCmd, "remove", "-y")
		} else {
			installCmd = append(installCmd, "install", "-y")
		}
	case "apk":
		installCmd = []string{"apk"}
		if state == "absent" {
			installCmd = append(installCmd, "del")
		} else {
			installCmd = append(installCmd, "add")
		}
	case "brew":
		installCmd = []string{"brew"}
		if state == "absent" {
			installCmd = append(installCmd, "uninstall")
		} else {
			installCmd = append(installCmd, "install")
		}
	default:
		return proto.Result{Status: "failed", Message: fmt.Sprintf("package.install: unsupported manager %q", manager)}
	}

	installCmd = append(installCmd, names...)

	// Execute via agent context
	execResult, err := executor.ExecuteCommand(context.Background(), installCmd, "", 5*time.Minute)
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
		Message: fmt.Sprintf("Package(s) %v now %s", names, state),
	}
}
