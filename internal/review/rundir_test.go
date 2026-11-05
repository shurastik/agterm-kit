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

func TestRunNumber(t *testing.T) {
	root := t.TempDir()
	t0 := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	var dirs []string
	for i := 0; i < 3; i++ {
		d, err := NewRunDir(root, t0.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, d)
	}
	same, _ := NewRunDir(root, t0.Add(2*time.Minute)) // collision suffix -2 sorts after its base
	os.WriteFile(filepath.Join(root, ".agk", "reviews", ".DS_Store"), nil, 0o644)
	for i, d := range append(dirs, same) {
		n, err := RunNumber(d)
		if err != nil || n != i+1 {
			t.Errorf("RunNumber(%s) = %d, %v; want %d", filepath.Base(d), n, err, i+1)
		}
	}
	// A newer run does not renumber an older one.
	NewRunDir(root, t0.Add(time.Hour))
	if n, _ := RunNumber(dirs[1]); n != 2 {
		t.Errorf("older run renumbered to %d", n)
	}
	if _, err := RunNumber(filepath.Join(root, ".agk", "reviews", "missing")); err == nil {
		t.Error("unknown dir must error")
	}
}
