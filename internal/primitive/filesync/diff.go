package filesync

import (
	"devopsctl/internal/proto"
)

// Diff computes the delta between srcTree (from the controller) and
// destTree (from the agent's detect phase).
//
//   - create: exists in src, missing in dest
//   - update: exists in both but SHA256 or size differs
//   - delete: exists in dest but not src (only populated if deleteExtra is true)
//   - chmod:  mode differs (file exists in both)
//   - chown:  uid or gid differs (file exists in both)
//   - mkdir:  directory needed in dest
func Diff(srcTree, destTree proto.FileTree, desiredMode uint32, desiredUID, desiredGID int, deleteExtra bool) proto.ChangeSet {
	var cs proto.ChangeSet

	// Determine which files need content transfer or metadata change.
	for rel, srcMeta := range srcTree {
		if srcMeta.IsDir {
			if _, exists := destTree[rel]; !exists {
				cs.Mkdir = append(cs.Mkdir, rel)
			}
			continue
		}

		destMeta, exists := destTree[rel]
		if !exists {
			cs.Create = append(cs.Create, rel)
		} else {
			// Content changed?
			if srcMeta.SHA256 != destMeta.SHA256 || srcMeta.Size != destMeta.Size {
				cs.Update = append(cs.Update, rel)
			}
		}

		// Metadata checks (only when desired values are specified).
		if desiredMode != 0 {
			effectiveMode := desiredMode
			if exists && destMeta.Mode != effectiveMode {
				cs.Chmod = appendUniq(cs.Chmod, rel)
			} else if !exists {
				// Will be set during apply — no separate chmod entry needed.
			}
		}
		if desiredUID >= 0 && desiredGID >= 0 {
			if exists && (destMeta.UID != desiredUID || destMeta.GID != desiredGID) {
				cs.Chown = appendUniq(cs.Chown, rel)
			}
		}
	}

	// Files that exist in dest but not src.
	if deleteExtra {
		for rel, destMeta := range destTree {
			if destMeta.IsDir {
				continue // handle dirs separately
			}
			if _, exists := srcTree[rel]; !exists {
				cs.Delete = append(cs.Delete, rel)
			}
		}
	}

	return cs
}

func appendUniq(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// IsEmpty returns true when the ChangeSet requires no action.
func IsEmpty(cs proto.ChangeSet) bool {
	return len(cs.Create) == 0 &&
		len(cs.Update) == 0 &&
		len(cs.Delete) == 0 &&
		len(cs.Chmod) == 0 &&
		len(cs.Chown) == 0 &&
		len(cs.Mkdir) == 0
}
