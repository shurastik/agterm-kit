package fsx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("keep"), 0o644)
	link := filepath.Join(dir, "out.json")
	os.Symlink(victim, link)
	if err := ReplaceFile(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("target overwritten: %q", b)
	}
	fi, _ := os.Lstat(link)
	if fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v", fi.Mode())
	}
	if b, _ := os.ReadFile(link); string(b) != "new" {
		t.Fatalf("content = %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left: %v", entries)
	}
}
