package filesync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"devopsctl/internal/proto"
)

const snapshotSuffix = ".devopsctl_snap"

// Apply executes the changeset on the local filesystem (on the agent).
// Files are received as chunks from the reader (conn), not buffered entirely.
// Safe order: mkdir → create/update files (streamed) → chmod → chown → delete.
func Apply(dest string, cs proto.ChangeSet, inputs map[string]string, chunkReader func() (*proto.ChunkMsg, error)) proto.Result {
	var result proto.Result
	snapshotDir := dest + snapshotSuffix + "_" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Parse optional metadata overrides.
	mode := parseMode(inputs["mode"])
	uid, gid := parseOwner(inputs["owner"], inputs["group"])

	// 1. Create directories.
	dirs := make([]string, len(cs.Mkdir))
	copy(dirs, cs.Mkdir)
	sort.Strings(dirs) // shortest first ensures parents before children
	for _, rel := range dirs {
		abs := filepath.Join(dest, rel)
		if err := os.MkdirAll(abs, 0755); err != nil {
			result.Failed = append(result.Failed, rel)
			result.Status = "partial"
		} else {
			result.Applied = append(result.Applied, rel)
		}
	}

	// 2. Stream create + update files from controller.
	needsTransfer := make(map[string]bool)
	for _, p := range cs.Create {
		needsTransfer[p] = true
	}
	for _, p := range cs.Update {
		needsTransfer[p] = true
	}

	var currentFile *os.File
	var currentPath string // relative path of currently-open file

	finishCurrentFile := func() error {
		if currentFile != nil {
			if err := currentFile.Close(); err != nil {
				return err
			}
			// Atomic rename: tmp → final
			finalPath := filepath.Join(dest, currentPath)
			if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
				return err
			}
			if err := os.Rename(currentFile.Name(), finalPath); err != nil {
				return err
			}
			// Apply mode immediately after write.
			if mode != 0 {
				_ = os.Chmod(finalPath, os.FileMode(mode))
			}
			if uid >= 0 && gid >= 0 {
				_ = syscall.Chown(finalPath, uid, gid)
			}
			result.Applied = append(result.Applied, currentPath)
			currentFile = nil
			currentPath = ""
		}
		return nil
	}

	for {
		chunk, err := chunkReader()
		if err == io.EOF || chunk == nil {
			break
		}
		if err != nil {
			result.Failed = append(result.Failed, "chunk-stream")
			result.Status = "partial"
			break
		}

		// Sentinel: empty path + EOF flag means the controller is done sending.
		if chunk.Path == "" && chunk.EOF {
			break
		}

		if chunk.Path != currentPath {
			// Switching to a new file — close previous.
			if err := finishCurrentFile(); err != nil {
				result.Failed = append(result.Failed, currentPath)
				result.Status = "partial"
			}
			if !needsTransfer[chunk.Path] {
				// Unexpected path — ignore.
				continue
			}
			// Snapshot existing file before overwriting.
			abs := filepath.Join(dest, chunk.Path)
			if _, statErr := os.Stat(abs); statErr == nil {
				snapPath := filepath.Join(snapshotDir, chunk.Path)
				_ = os.MkdirAll(filepath.Dir(snapPath), 0755)
				_ = copyFileSimple(abs, snapPath)
			}
			// Create temp file in same dir for atomic replace.
			tmpDir := filepath.Join(dest, filepath.Dir(chunk.Path))
			_ = os.MkdirAll(tmpDir, 0755)
			tmp, tmpErr := os.CreateTemp(tmpDir, ".devopsctl_tmp_*")
			if tmpErr != nil {
				result.Failed = append(result.Failed, chunk.Path)
				result.Status = "partial"
				continue
			}
			currentFile = tmp
			currentPath = chunk.Path
		}

		if currentFile != nil && len(chunk.Data) > 0 {
			if _, err := currentFile.Write(chunk.Data); err != nil {
				result.Failed = append(result.Failed, chunk.Path)
				result.Status = "partial"
			}
		}

		if chunk.EOF {
			if err := finishCurrentFile(); err != nil {
				result.Failed = append(result.Failed, currentPath)
				result.Status = "partial"
			}
		}
	}
	// Ensure last file is closed.
	_ = finishCurrentFile()

	// 3. chmod paths that exist and need mode changes.
	if mode != 0 {
		for _, rel := range cs.Chmod {
			abs := filepath.Join(dest, rel)
			if err := os.Chmod(abs, os.FileMode(mode)); err != nil {
				result.Failed = append(result.Failed, rel)
				result.Status = "partial"
			} else {
				result.Applied = appendUniq(result.Applied, rel)
			}
		}
	}

	// 4. chown paths.
	if uid >= 0 && gid >= 0 {
		for _, rel := range cs.Chown {
			abs := filepath.Join(dest, rel)
			if err := syscall.Chown(abs, uid, gid); err != nil {
				result.Failed = append(result.Failed, rel)
				result.Status = "partial"
			}
		}
	}

	// 5. Delete extra files (optional, only if in changeset).
	for _, rel := range cs.Delete {
		abs := filepath.Join(dest, rel)
		// Snapshot before delete.
		snapPath := filepath.Join(snapshotDir, rel)
		_ = os.MkdirAll(filepath.Dir(snapPath), 0755)
		_ = copyFileSimple(abs, snapPath)
		if err := os.Remove(abs); err != nil {
			result.Failed = append(result.Failed, rel)
			result.Status = "partial"
		} else {
			result.Applied = append(result.Applied, rel)
		}
	}

	// Persist snapshot dir path for rollback.
	if snapshotDirHasContent(snapshotDir) {
		markerPath := filepath.Join(dest, ".devopsctl_last_snap")
		_ = os.WriteFile(markerPath, []byte(snapshotDir), 0600)
	}

	if len(result.Failed) > 0 {
		if len(result.Applied) > 0 {
			result.Status = "partial"
			result.Message = fmt.Sprintf("applied %d, failed %d", len(result.Applied), len(result.Failed))
		} else {
			result.Status = "failed"
			result.Message = fmt.Sprintf("failed %d", len(result.Failed))
		}
	} else {
		result.Status = "success"
		result.Message = fmt.Sprintf("applied %d changes", len(result.Applied))
	}
	return result
}

// --- helpers ---

func parseMode(s string) uint32 {
	if s == "" {
		return 0
	}
	// Accept "0755" octal string.
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "0"), 8, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

func parseOwner(owner, group string) (int, int) {
	if owner == "" || group == "" {
		return -1, -1
	}
	// For MVP, accept numeric uid/gid only.
	uid, err1 := strconv.Atoi(owner)
	gid, err2 := strconv.Atoi(group)
	if err1 != nil || err2 != nil {
		return -1, -1
	}
	return uid, gid
}

func copyFileSimple(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func snapshotDirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}
