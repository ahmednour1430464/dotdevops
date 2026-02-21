package filesync

import (
	"fmt"
	"os"
	"path/filepath"

	"devopsctl/internal/proto"
)

// Rollback attempts to restore the destination to the state before the last
// apply call. It reads the snapshot directory from the marker file written
// by Apply.
//
// Strategy (in order):
//  1. Restore snapshotted files from snapshot dir back to dest.
//  2. Remove any newly created files (those in cs.Create) that have no snapshot.
//  3. Mark result as partial if anything fails.
func Rollback(dest string, cs proto.ChangeSet) proto.Result {
	var result proto.Result

	markerPath := filepath.Join(dest, ".devopsctl_last_snap")
	snapDirBytes, err := os.ReadFile(markerPath)
	if err != nil {
		// No snapshot available.
		result.Message = "rollback skipped: no snapshot found"
		result.Status = "success" // Not a failure — nothing to undo.
		return result
	}
	snapshotDir := string(snapDirBytes)

	// Restore updated/deleted files from snapshot.
	toRestore := append(cs.Update, cs.Delete...)
	for _, rel := range toRestore {
		snapPath := filepath.Join(snapshotDir, rel)
		destPath := filepath.Join(dest, rel)
		if _, err := os.Stat(snapPath); os.IsNotExist(err) {
			// No snapshot for this file — skip.
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			result.Failed = append(result.Failed, rel)
			result.Status = "partial"
			continue
		}
		if err := copyFileSimple(snapPath, destPath); err != nil {
			result.Failed = append(result.Failed, rel)
			result.Status = "partial"
		} else {
			result.Applied = append(result.Applied, rel)
		}
	}

	// Remove newly created files (they have no prior version to restore).
	for _, rel := range cs.Create {
		destPath := filepath.Join(dest, rel)
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			result.Failed = append(result.Failed, rel)
			result.Status = "partial"
		} else {
			result.Applied = append(result.Applied, rel)
		}
	}

	// Clean up snapshot dir.
	_ = os.RemoveAll(snapshotDir)
	_ = os.Remove(markerPath)

	if len(result.Failed) > 0 {
		if len(result.Applied) > 0 {
			result.Status = "partial"
			result.Message = fmt.Sprintf("rollback partial: restored %d, failed %d", len(result.Applied), len(result.Failed))
		} else {
			result.Status = "failed"
			result.Message = fmt.Sprintf("rollback failed: %d files failed", len(result.Failed))
		}
	} else {
		result.Status = "success"
		result.Message = fmt.Sprintf("rollback complete: restored %d files", len(result.Applied))
	}
	return result
}
