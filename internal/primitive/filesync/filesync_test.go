package filesync_test

import (
	"os"
	"path/filepath"
	"testing"

	"devopsctl/internal/primitive/filesync"
	"devopsctl/internal/proto"
)

// TestDetectEmpty verifies that Detect on a non-existent dir returns an empty tree.
func TestDetectEmpty(t *testing.T) {
	tree, err := filesync.Detect("/tmp/devopsctl_test_nonexistent_dir_xyz")
	if err != nil {
		t.Fatalf("expected no error for missing dir, got: %v", err)
	}
	if len(tree) != 0 {
		t.Fatalf("expected empty tree, got %d entries", len(tree))
	}
}

// TestDetectAndDiff covers the full detect → diff cycle.
func TestDetectAndDiff(t *testing.T) {
	// Create a temp source directory.
	src := t.TempDir()
	dst := t.TempDir()

	// Write source files.
	writeFile(t, filepath.Join(src, "index.html"), "hello world")
	writeFile(t, filepath.Join(src, "app.js"), "console.log('hi')")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "sub", "style.css"), "body{}")

	// Both source and dest empty initially → all should be created.
	srcTree, err := filesync.BuildSourceTree(src)
	if err != nil {
		t.Fatalf("BuildSourceTree: %v", err)
	}
	dstTree, err := filesync.Detect(dst)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	cs := filesync.Diff(srcTree, dstTree, 0, -1, -1, false)

	if len(cs.Create) != 3 {
		t.Errorf("expected 3 creates (2 files + 1 in sub), got %d: %v", len(cs.Create), cs.Create)
	}
	if len(cs.Mkdir) != 1 {
		t.Errorf("expected 1 mkdir (sub), got %d: %v", len(cs.Mkdir), cs.Mkdir)
	}
	if len(cs.Update) != 0 {
		t.Errorf("expected 0 updates, got %d", len(cs.Update))
	}

	// Now simulate a second run with synced dest → should be empty diff.
	for k, meta := range srcTree {
		dstTree[k] = meta
	}
	cs2 := filesync.Diff(srcTree, dstTree, 0, -1, -1, false)
	if !filesync.IsEmpty(cs2) {
		t.Errorf("expected empty diff on second run, got %+v", cs2)
	}
}

// TestDiffDetectsUpdate verifies that a changed file is detected as an update.
func TestDiffDetectsUpdate(t *testing.T) {
	srcTree := proto.FileTree{
		"index.html": {Path: "index.html", Size: 10, SHA256: "aabbcc", Mode: 0644},
	}
	dstTree := proto.FileTree{
		"index.html": {Path: "index.html", Size: 9, SHA256: "xxyyzz", Mode: 0644},
	}
	cs := filesync.Diff(srcTree, dstTree, 0, -1, -1, false)
	if len(cs.Update) != 1 || cs.Update[0] != "index.html" {
		t.Errorf("expected update for index.html, got %+v", cs)
	}
}

// TestDiffDeleteExtra verifies delete_extra behaviour.
func TestDiffDeleteExtra(t *testing.T) {
	srcTree := proto.FileTree{}
	dstTree := proto.FileTree{
		"old.html": {Path: "old.html", Size: 5, SHA256: "deadbeef"},
	}
	cs := filesync.Diff(srcTree, dstTree, 0, -1, -1, true)
	if len(cs.Delete) != 1 || cs.Delete[0] != "old.html" {
		t.Errorf("expected delete for old.html, got %+v", cs)
	}
}

// TestIsEmpty verifies the empty-changeset detector.
func TestIsEmpty(t *testing.T) {
	if !filesync.IsEmpty(proto.ChangeSet{}) {
		t.Error("empty ChangeSet should be IsEmpty")
	}
	if filesync.IsEmpty(proto.ChangeSet{Create: []string{"a"}}) {
		t.Error("non-empty ChangeSet should not be IsEmpty")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}
