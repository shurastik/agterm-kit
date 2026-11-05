package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRunDir(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 5, 14, 3, 9, 0, time.Local)
	a, err := NewRunDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".agk", "reviews", "20261105-140309"); a != want {
		t.Fatalf("dir = %q, want %q", a, want)
	}
	b, err := NewRunDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewRunDir(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if b != a+"-2" || c != a+"-3" {
		t.Fatalf("collisions: %q %q", b, c)
	}
	for _, d := range []string{a, b, c} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("%s not created: %v", d, err)
		}
	}
}
