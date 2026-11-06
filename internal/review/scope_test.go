package review

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo creates a repo with one commit on main and returns its root.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "a.go", "package a\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "init")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectBranchPlusUncommitted(t *testing.T) {
	dir := gitRepo(t)
	git(t, dir, "checkout", "-qb", "feat")
	write(t, dir, "b.go", "package a\n\nfunc Committed() {}\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "b")
	write(t, dir, "a.go", "package a\n\nfunc Uncommitted() {}\n")
	write(t, dir, "staged.go", "package a\n\nfunc Staged() {}\n")
	git(t, dir, "add", "staged.go")
	write(t, dir, "new/untracked.go", "package a\n\nfunc Untracked() {}\n")

	sub := filepath.Join(dir, "new")
	s, err := Collect(context.Background(), sub, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != dir {
		t.Fatalf("root = %q, want %q", s.Root, dir)
	}
	for _, want := range []string{"func Committed", "func Uncommitted", "func Staged"} {
		if !strings.Contains(s.Diff, want) {
			t.Errorf("diff lacks %q:\n%s", want, s.Diff)
		}
	}
	if !strings.Contains(s.Stat, "b.go") {
		t.Errorf("stat lacks b.go: %q", s.Stat)
	}
	if len(s.Untracked) != 1 || s.Untracked[0].Path != "new/untracked.go" || !strings.Contains(s.Untracked[0].Content, "func Untracked") {
		t.Fatalf("untracked = %+v", s.Untracked)
	}
}

func TestCollectExcludesIgnoredAndAgkDir(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, ".gitignore", "secret.env\n")
	git(t, dir, "add", ".gitignore")
	git(t, dir, "commit", "-qm", "ignore")
	write(t, dir, "secret.env", "TOKEN=x\n")
	write(t, dir, ".agk/reviews/1/prompt.md", "old prompt\n")
	write(t, dir, "keep.txt", "keep\n")

	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Untracked) != 1 || s.Untracked[0].Path != "keep.txt" {
		t.Fatalf("untracked = %+v", s.Untracked)
	}
}

func TestCollectNothingToReview(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, ".agk/reviews/1/prompt.md", "old\n")
	_, err := Collect(context.Background(), dir, "main")
	if !errors.Is(err, ErrNothingToReview) || err.Error() != "nothing to review" {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectOmitsBinaryLargeAndSymlinks(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, "bin.dat", "ab\x00cd")
	write(t, dir, "big.txt", strings.Repeat("x", MaxUntrackedBytes+1))
	outside := filepath.Join(t.TempDir(), "secret")
	write(t, filepath.Dir(outside), "secret", "PRIVATE KEY\n")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]UntrackedFile{}
	for _, u := range s.Untracked {
		got[u.Path] = u
	}
	for _, p := range []string{"bin.dat", "big.txt", "link"} {
		u, ok := got[p]
		if !ok {
			t.Fatalf("%s missing from %+v", p, s.Untracked)
		}
		if u.Content != "" || u.Omitted == "" {
			t.Errorf("%s: content %q omitted %q, want omitted with reason", p, u.Content, u.Omitted)
		}
	}
	if strings.Contains(got["link"].Omitted, "PRIVATE") {
		t.Fatal("symlink target content leaked")
	}
}

func TestCollectBadBase(t *testing.T) {
	dir := gitRepo(t)
	if _, err := Collect(context.Background(), dir, "no-such-branch"); err == nil {
		t.Fatal("want error")
	}
}

// A textconv driver can hide changes from `git diff`; the review must see the
// raw change, so a converter that drops the edited line must not matter.
func TestCollectIgnoresTextconv(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, ".gitattributes", "*.cfg diff=hide\n")
	write(t, dir, "app.cfg", "keep\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "cfg")
	git(t, dir, "config", "diff.hide.textconv", "grep -v SECRET_CHANGE")
	write(t, dir, "app.cfg", "keep\nSECRET_CHANGE\n")
	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("change hidden by textconv: %v", err)
	}
	if !strings.Contains(s.Diff, "+SECRET_CHANGE") {
		t.Fatalf("diff lacks the raw change:\n%s", s.Diff)
	}
}

// Git config must not hide a submodule update from the review.
func TestCollectSeesSubmoduleUpdateDespiteIgnoreConfig(t *testing.T) {
	sub := gitRepo(t)
	dir := gitRepo(t)
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "dep")
	git(t, dir, "commit", "-qm", "add dep")
	git(t, dir, "config", "diff.ignoreSubmodules", "all")
	git(t, dir, "config", "-f", ".gitmodules", "submodule.dep.ignore", "all")
	write(t, filepath.Join(dir, "dep"), "new.go", "package a\n")
	git(t, filepath.Join(dir, "dep"), "add", "-A")
	git(t, filepath.Join(dir, "dep"), "commit", "-qm", "bump")
	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("submodule update hidden: %v", err)
	}
	if !strings.Contains(s.Diff, "Subproject commit") || !strings.Contains(s.Stat, "dep") {
		t.Fatalf("diff/stat lack the gitlink change:\n%s\n%s", s.Diff, s.Stat)
	}
}
