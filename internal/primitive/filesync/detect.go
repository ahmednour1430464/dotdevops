// Package filesync implements the file.sync primitive.
// detect.go walks the destination directory and returns a FileTree snapshot.
package filesync

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"devopsctl/internal/proto"
)

const chunkSize = 256 * 1024 // 256 KB streaming chunk

// Detect walks dir (the destination) and returns a FileTree representing
// every file and directory currently present. Hashes are computed by
// streaming — no full-file buffering.
func Detect(dir string) (proto.FileTree, error) {
	tree := make(proto.FileTree)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Destination may not exist yet — that is a valid empty state.
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("walking %s: %w", path, err)
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		// Normalise separators to forward slash for cross-platform consistency.
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil // skip the root itself
		}

		meta := proto.FileMeta{
			Path:  rel,
			IsDir: info.IsDir(),
			Mode:  uint32(info.Mode().Perm()),
		}

		// Populate uid/gid on systems that support it (Linux agent).
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			meta.UID = int(stat.Uid)
			meta.GID = int(stat.Gid)
		}

		if !info.IsDir() {
			meta.Size = info.Size()
			hash, hashErr := hashFile(path)
			if hashErr != nil {
				return fmt.Errorf("hashing %s: %w", path, hashErr)
			}
			meta.SHA256 = hash
		}

		tree[rel] = meta
		return nil
	})

	return tree, err
}

// hashFile streams a file through SHA-256 without loading it into memory.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, chunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// BuildSourceTree walks a local source directory (on the controller side)
// and returns a FileTree for diffing. Identical to Detect but used on
// the source rather than the destination.
func BuildSourceTree(src string) (proto.FileTree, error) {
	// Strip any trailing path separator for clean relative paths.
	src = strings.TrimRight(src, string(filepath.Separator))
	return Detect(src)
}
