package controller

import (
	"fmt"

	"devopsctl/internal/primitive/filesync"
	"devopsctl/internal/proto"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

// PrintDiff prints a human-readable summary of the changeset.
func PrintDiff(nodeID, targetID string, cs proto.ChangeSet) {
	if filesync.IsEmpty(cs) {
		return
	}
	fmt.Printf("\n── diff: %s → %s ──────────────────────────────\n", nodeID, targetID)
	for _, p := range cs.Mkdir {
		fmt.Printf("  %s+%s dir  %s\n", colorGreen, colorReset, p)
	}
	for _, p := range cs.Create {
		fmt.Printf("  %s+%s file %s\n", colorGreen, colorReset, p)
	}
	for _, p := range cs.Update {
		fmt.Printf("  %s~%s file %s\n", colorYellow, colorReset, p)
	}
	for _, p := range cs.Chmod {
		fmt.Printf("  %s~%s mode %s\n", colorCyan, colorReset, p)
	}
	for _, p := range cs.Chown {
		fmt.Printf("  %s~%s own  %s\n", colorCyan, colorReset, p)
	}
	for _, p := range cs.Delete {
		fmt.Printf("  %s-%s file %s\n", colorRed, colorReset, p)
	}
	fmt.Println()
}
