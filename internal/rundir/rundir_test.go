package rundir

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 5, 14, 3, 9, 0, time.Local)
	a, err := New(root, Reviews, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".agk", "reviews", "20261105-140309"); a != want {
		t.Fatalf("dir = %q, want %q", a, want)
	}
	b, err := New(root, Reviews, now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(root, Reviews, now)
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
	ap, err := New(root, Applies, now)
	if err != nil || ap != filepath.Join(root, ".agk", "applies", "20261105-140309") {
		t.Fatalf("applies dir = %q, %v", ap, err)
	}
	if _, err := New(root, "other", now); err == nil {
		t.Fatal("unknown kind must error")
	}
}

func TestNumber(t *testing.T) {
	root := t.TempDir()
	t0 := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	var dirs []string
	for i := 0; i < 3; i++ {
		d, err := New(root, Reviews, t0.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, d)
	}
	same, _ := New(root, Reviews, t0.Add(2*time.Minute)) // collision suffix -2 sorts after its base
	os.WriteFile(filepath.Join(root, ".agk", "reviews", ".DS_Store"), nil, 0o644)
	for i, d := range append(dirs, same) {
		n, err := Number(d)
		if err != nil || n != i+1 {
			t.Errorf("Number(%s) = %d, %v; want %d", filepath.Base(d), n, err, i+1)
		}
	}
	// A newer run does not renumber an older one.
	New(root, Reviews, t0.Add(time.Hour))
	if n, _ := Number(dirs[1]); n != 2 {
		t.Errorf("older run renumbered to %d", n)
	}
	if _, err := Number(filepath.Join(root, ".agk", "reviews", "missing")); err == nil {
		t.Error("unknown dir must error")
	}
	// Numbering is per kind.
	ap, _ := New(root, Applies, t0.Add(2*time.Hour))
	if n, err := Number(ap); err != nil || n != 1 {
		t.Errorf("first apply run = %d, %v; want 1", n, err)
	}
}

func TestRoot(t *testing.T) {
	root := t.TempDir()
	d, _ := New(root, Applies, time.Now())
	if got, err := Root(d, Applies); err != nil || got != root {
		t.Fatalf("Root = %q, %v", got, err)
	}
	if _, err := Root(d, Reviews); err == nil {
		t.Fatal("apply dir accepted as review dir")
	}
	if _, err := Root(root, Applies); err == nil {
		t.Fatal("repo root accepted as run dir")
	}
}

func TestRounds(t *testing.T) {
	d, _ := New(t.TempDir(), Reviews, time.Now())
	if _, err := Round(d); err == nil {
		t.Fatal("missing round file must error")
	}
	rd, err := MakeRound(d, 1)
	if err != nil || rd != filepath.Join(d, "rounds", "1") || RoundDir(d, 1) != rd {
		t.Fatalf("MakeRound = %q, %v", rd, err)
	}
	if fi, err := os.Stat(rd); err != nil || !fi.IsDir() {
		t.Fatalf("round dir not created: %v", err)
	}
	if err := SetRound(d, 2); err != nil {
		t.Fatal(err)
	}
	if k, err := Round(d); err != nil || k != 2 {
		t.Fatalf("Round = %d, %v", k, err)
	}
	os.WriteFile(filepath.Join(d, RoundFile), []byte("zero\n"), 0o644)
	if _, err := Round(d); err == nil {
		t.Fatal("garbage round must error")
	}
	os.WriteFile(filepath.Join(d, RoundFile), []byte("0\n"), 0o644)
	if _, err := Round(d); err == nil {
		t.Fatal("round 0 must error")
	}
}

func TestJSON(t *testing.T) {
	d := t.TempDir()
	type rec struct {
		A string `json:"a"`
	}
	var got rec
	if have, err := ReadJSON(d, "w.json", &got); have || err != nil {
		t.Fatalf("missing file: have=%v err=%v", have, err)
	}
	if err := WriteJSON(d, "w.json", rec{A: "x"}); err != nil {
		t.Fatal(err)
	}
	if have, err := ReadJSON(d, "w.json", &got); !have || err != nil || got.A != "x" {
		t.Fatalf("round trip: %+v %v %v", got, have, err)
	}
	os.WriteFile(filepath.Join(d, "w.json"), []byte("{"), 0o644)
	if _, err := ReadJSON(d, "w.json", &got); err == nil {
		t.Fatal("bad json must error")
	}
}
