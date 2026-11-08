package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// featRepo is gitRepo on a feature branch, so committing work does not move the
// merge base with main.
func featRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)
	git(t, dir, "checkout", "-qb", "feat")
	return dir
}

func fingerprint(t *testing.T, dir string) string {
	t.Helper()
	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Fingerprint) != 64 {
		t.Fatalf("fingerprint = %q, want 64 hex digits", s.Fingerprint)
	}
	return s.Fingerprint
}

func wantSame(t *testing.T, what, a, b string) {
	t.Helper()
	if a != b {
		t.Errorf("%s: fingerprint changed, want it unchanged", what)
	}
}

func wantDifferent(t *testing.T, what, a, b string) {
	t.Helper()
	if a == b {
		t.Errorf("%s: fingerprint unchanged, want it to change", what)
	}
}

func TestFingerprintStableAcrossCommit(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "new.go", "package a\n\nfunc New() {}\n")
	before := fingerprint(t, dir)
	git(t, dir, "add", "-A")
	wantSame(t, "staged", before, fingerprint(t, dir))
	git(t, dir, "commit", "-qm", "new")
	wantSame(t, "committed", before, fingerprint(t, dir))
}

func TestFingerprintStableAcrossRenameStaging(t *testing.T) {
	dir := featRepo(t)
	if err := os.Rename(filepath.Join(dir, "a.go"), filepath.Join(dir, "c.go")); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, dir)
	git(t, dir, "add", "-A")
	wantSame(t, "staged rename", before, fingerprint(t, dir))
	git(t, dir, "commit", "-qm", "rename")
	wantSame(t, "committed rename", before, fingerprint(t, dir))
}

func TestFingerprintChangesOnContentEdit(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "new.go", "package a // 1\n")
	before := fingerprint(t, dir)
	write(t, dir, "new.go", "package a // 2\n")
	wantDifferent(t, "one-byte edit", before, fingerprint(t, dir))

	// A tracked file edited again in place.
	write(t, dir, "a.go", "package a // x\n")
	tracked := fingerprint(t, dir)
	write(t, dir, "a.go", "package a // y\n")
	wantDifferent(t, "second edit of a tracked file", tracked, fingerprint(t, dir))
}

func TestFingerprintChangesOnPathSet(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "one.go", "package a\n")
	before := fingerprint(t, dir)
	write(t, dir, "two.go", "package a\n")
	withTwo := fingerprint(t, dir)
	wantDifferent(t, "added untracked file", before, withTwo)
	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}
	wantDifferent(t, "deleted tracked file", withTwo, fingerprint(t, dir))
}

func TestFingerprintChangesWhenMergeBaseMoves(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "new.go", "package a\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "new")
	before := fingerprint(t, dir)
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "m.go", "package a\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "main moves")
	git(t, dir, "checkout", "-q", "feat")
	git(t, dir, "merge", "-q", "--no-edit", "main")
	wantDifferent(t, "merge base moved", before, fingerprint(t, dir))
}

func TestFingerprintChangesOnModeAndKind(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "x.txt", "target")
	before := fingerprint(t, dir)

	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterChmod := fingerprint(t, dir)
	wantDifferent(t, "dropped executable bit", before, afterChmod)

	if err := os.Remove(filepath.Join(dir, "x.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(dir, "x.txt")); err != nil {
		t.Fatal(err)
	}
	wantDifferent(t, "file replaced by symlink with the same text", afterChmod, fingerprint(t, dir))
}

func TestFingerprintCoversOmittedUntrackedFiles(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "bin.dat", "ab\x00cd")
	write(t, dir, "big.txt", strings.Repeat("x", MaxUntrackedBytes+1))
	if err := os.Symlink("one", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, dir)

	write(t, dir, "bin.dat", "ab\x00ce")
	afterBin := fingerprint(t, dir)
	wantDifferent(t, "binary file edited", before, afterBin)

	write(t, dir, "big.txt", strings.Repeat("x", MaxUntrackedBytes)+"y")
	afterBig := fingerprint(t, dir)
	wantDifferent(t, "large file edited", afterBin, afterBig)

	if err := os.Remove(filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("two", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	wantDifferent(t, "symlink retargeted", afterBig, fingerprint(t, dir))
}

// subRepo returns a superproject on a feature branch with a submodule "dep"
// holding a tracked a.go, and the submodule's directory.
func subRepo(t *testing.T) (dir, sub string) {
	t.Helper()
	upstream := gitRepo(t)
	dir = gitRepo(t)
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", upstream, "dep")
	git(t, dir, "commit", "-qm", "add dep")
	git(t, dir, "checkout", "-qb", "feat")
	sub = filepath.Join(dir, "dep")
	write(t, sub, "bump.go", "package a\n")
	git(t, sub, "add", "-A")
	git(t, sub, "commit", "-qm", "bump")
	return dir, sub
}

func TestFingerprintSubmodule(t *testing.T) {
	dir, sub := subRepo(t)
	before := fingerprint(t, dir)

	write(t, sub, "more.go", "package a\n")
	git(t, sub, "add", "-A")
	git(t, sub, "commit", "-qm", "more")
	afterCommit := fingerprint(t, dir)
	wantDifferent(t, "new commit in the submodule", before, afterCommit)

	write(t, sub, "a.go", "package a // 1\n")
	dirty := fingerprint(t, dir)
	wantDifferent(t, "uncommitted edit in the submodule", afterCommit, dirty)

	// Same HEAD, same status lines: only the content differs.
	write(t, sub, "a.go", "package a // 2\n")
	secondEdit := fingerprint(t, dir)
	wantDifferent(t, "second edit of a modified tracked file", dirty, secondEdit)

	write(t, sub, "new.go", "package a // 1\n")
	untracked := fingerprint(t, dir)
	wantDifferent(t, "untracked file in the submodule", secondEdit, untracked)
	write(t, sub, "new.go", "package a // 2\n")
	wantDifferent(t, "second edit of an untracked file", untracked, fingerprint(t, dir))
}

func TestFingerprintUninitializedSubmodule(t *testing.T) {
	dir, sub := subRepo(t)
	git(t, dir, "add", "dep")
	before := fingerprint(t, dir)
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	wantDifferent(t, "submodule uninitialized", before, s.Fingerprint)
}

// An uninitialized submodule is still a dependency version: the gitlink SHA
// is part of its state.
func TestFingerprintUninitializedSubmoduleGitlinkSHA(t *testing.T) {
	dir, sub := subRepo(t)
	sha1 := strings.TrimSpace(git(t, sub, "rev-parse", "HEAD"))
	write(t, sub, "more.go", "package a\n")
	git(t, sub, "add", "-A")
	git(t, sub, "commit", "-qm", "more")
	sha2 := strings.TrimSpace(git(t, sub, "rev-parse", "HEAD"))
	git(t, dir, "update-index", "--cacheinfo", "160000,"+sha1+",dep")
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	first := fingerprint(t, dir)
	git(t, dir, "update-index", "--cacheinfo", "160000,"+sha2+",dep")
	wantDifferent(t, "gitlink SHA moved while uninitialized", first, fingerprint(t, dir))
}

func TestFingerprintChangesOnOwnerExecuteBitOnly(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "run.sh")
	git(t, dir, "commit", "-qm", "script")
	write(t, dir, "run.sh", "#!/bin/sh\n# edit\n") // in scope against main
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, dir)
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o655); err != nil {
		t.Fatal(err)
	}
	wantDifferent(t, "chmod u-x (0755 -> 0655)", before, fingerprint(t, dir))
	// Group/other bits alone do not change git's mode.
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o744); err != nil {
		t.Fatal(err)
	}
	owner := fingerprint(t, dir)
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	wantSame(t, "group/other execute bits", owner, fingerprint(t, dir))
}

// With core.filemode=false git takes a tracked file's executable bit from the
// index, so a mode change by update-index shows in the diff while the working
// file stays as it was.
func TestFingerprintFilemodeFalseUsesIndexMode(t *testing.T) {
	dir := featRepo(t)
	git(t, dir, "config", "core.filemode", "false")
	write(t, dir, "a.go", "package a // modified\n")
	before := fingerprint(t, dir)

	git(t, dir, "update-index", "--chmod=+x", "a.go")
	after := fingerprint(t, dir)
	wantDifferent(t, "update-index --chmod=+x with core.filemode=false", before, after)

	git(t, dir, "commit", "-qm", "mode only")
	wantSame(t, "committed mode change", after, fingerprint(t, dir))
}

// With core.filemode=false git stores a new file as 100644 whatever its
// permissions, so staging or committing it must not change the fingerprint.
func TestFingerprintFilemodeFalseNewExecutableFile(t *testing.T) {
	dir := featRepo(t)
	git(t, dir, "config", "core.filemode", "false")
	write(t, dir, "new.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "new.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, dir)
	git(t, dir, "add", "new.sh")
	wantSame(t, "staged new executable file", before, fingerprint(t, dir))
	git(t, dir, "commit", "-qm", "new script")
	wantSame(t, "committed new executable file", before, fingerprint(t, dir))
}

// A skip-worktree file missing on disk is diffed by its index content, so two
// index versions are two fingerprints; it is not a deletion.
func TestFingerprintSparseCheckoutHiddenFile(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "hidden.txt", "h1\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "hidden")
	git(t, dir, "sparse-checkout", "init", "--no-cone")
	git(t, dir, "sparse-checkout", "set", "/a.go")
	if _, err := os.Stat(filepath.Join(dir, "hidden.txt")); err == nil {
		t.Fatal("sparse checkout left hidden.txt on disk")
	}
	first := fingerprint(t, dir)
	setIndex := func(content string) {
		cmd := exec.Command("git", "hash-object", "-w", "--stdin")
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(content)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		git(t, dir, "update-index", "--cacheinfo", "100644,"+strings.TrimSpace(string(out))+",hidden.txt")
		git(t, dir, "update-index", "--skip-worktree", "hidden.txt")
	}
	setIndex("h2\n")
	second := fingerprint(t, dir)
	wantDifferent(t, "hidden file, index content changed", first, second)
	setIndex("h3\n")
	wantDifferent(t, "hidden file, index content changed again", second, fingerprint(t, dir))
}

// Paths with glob characters are literal paths, not pathspecs: the index
// lookup of "run[1].sh" must not land on the entry of "run1.sh".
func TestFingerprintFilemodeFalseGlobPath(t *testing.T) {
	dir := featRepo(t)
	git(t, dir, "config", "core.filemode", "false")
	write(t, dir, "run1.sh", "#!/bin/sh\n")
	write(t, dir, "run[1].sh", "#!/bin/sh\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "scripts")
	write(t, dir, "run1.sh", "#!/bin/sh\n# edit\n")
	write(t, dir, "run[1].sh", "#!/bin/sh\n# edit\n")
	before := fingerprint(t, dir)
	git(t, dir, "update-index", "--chmod=+x", "run[1].sh")
	wantDifferent(t, "update-index --chmod=+x on run[1].sh", before, fingerprint(t, dir))
}

func TestFingerprintUninitializedSubmoduleGlobPath(t *testing.T) {
	upstream := gitRepo(t)
	dir := gitRepo(t)
	for _, name := range []string{"dep[1]", "dep1"} { // "dep[1]" first: as a pathspec it would match dep1
		git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", upstream, name)
	}
	git(t, dir, "commit", "-qm", "add deps")
	git(t, dir, "checkout", "-qb", "feat")
	var shas []string
	for _, f := range []string{"one.go", "two.go"} {
		write(t, upstream, f, "package a\n")
		git(t, upstream, "add", "-A")
		git(t, upstream, "commit", "-qm", f)
		shas = append(shas, strings.TrimSpace(git(t, upstream, "rev-parse", "HEAD")))
	}
	for _, name := range []string{"dep1", "dep[1]"} {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "update-index", "--cacheinfo", "160000,"+shas[0]+",dep1")
	git(t, dir, "update-index", "--cacheinfo", "160000,"+shas[0]+",dep[1]")
	first := fingerprint(t, dir)
	git(t, dir, "update-index", "--cacheinfo", "160000,"+shas[1]+",dep[1]")
	wantDifferent(t, "gitlink SHA of dep[1] moved while uninitialized", first, fingerprint(t, dir))
}

func TestFingerprintConsistentSnapshot(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "new.go", "package a // old\n")

	calls := 0
	afterFirstFingerprint = func() {
		calls++
		if calls == 1 {
			write(t, dir, "new.go", "package a // edited\n")
		}
	}
	t.Cleanup(func() { afterFirstFingerprint = nil })

	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("collected %d times, want 2", calls)
	}
	if len(s.Untracked) != 1 || !strings.Contains(s.Untracked[0].Content, "edited") {
		t.Fatalf("untracked = %+v, want the edited content", s.Untracked)
	}
	afterFirstFingerprint = nil
	if want := fingerprint(t, dir); s.Fingerprint != want {
		t.Errorf("fingerprint %s does not match the edited tree %s", s.Fingerprint, want)
	}
}

func TestFingerprintFilesKeepChanging(t *testing.T) {
	dir := featRepo(t)
	write(t, dir, "new.go", "package a\n")

	n := 0
	calls := 0
	afterFirstFingerprint = func() {
		calls++
		n++
		write(t, dir, "new.go", "package a // "+strings.Repeat("x", n)+"\n")
	}
	t.Cleanup(func() { afterFirstFingerprint = nil })

	_, err := Collect(context.Background(), dir, "main")
	if err == nil || !strings.Contains(err.Error(), "changed during collection") {
		t.Fatalf("err = %v, want a changed during collection error", err)
	}
	if calls != 3 {
		t.Errorf("attempts = %d, want 3", calls)
	}
}
