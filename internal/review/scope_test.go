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

func TestCollectOmitsNonUTF8Untracked(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, "legacy.txt", "\xcf\xf0\xe8\xe2\xe5\xf2\n") // cp1251, no NUL
	s, err := Collect(context.Background(), dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Untracked) != 1 || s.Untracked[0].Content != "" || !strings.Contains(s.Untracked[0].Omitted, "UTF-8") {
		t.Fatalf("untracked = %+v", s.Untracked)
	}
}

// planFiles maps PlanScope file paths to their entries.
func planFiles(fs []UntrackedFile) map[string]UntrackedFile {
	m := map[string]UntrackedFile{}
	for _, f := range fs {
		m[f.Path] = f
	}
	return m
}

func planPaths(fs []UntrackedFile) []string {
	var ps []string
	for _, f := range fs {
		ps = append(ps, f.Path)
	}
	return ps
}

func TestCollectPlan(t *testing.T) {
	const ch = "openspec/changes/add-x/"
	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		files     []string
		mainSpecs []string
		check     func(t *testing.T, s PlanScope)
	}{
		{
			name: "untracked change on a dirty tree",
			setup: func(t *testing.T, dir string) {
				write(t, dir, "a.go", "package a\n\nfunc UnrelatedEdit() {}\n")
				write(t, dir, ch+"proposal.md", "# Proposal X\n")
				write(t, dir, ch+"tasks.md", "- [ ] 1.1 do x\n")
				write(t, dir, ch+".openspec.yaml", "schema: spec-driven\n")
			},
			files: []string{ch + ".openspec.yaml", ch + "proposal.md", ch + "tasks.md"},
			check: func(t *testing.T, s PlanScope) {
				for _, f := range append(s.Files, s.MainSpecs...) {
					if strings.Contains(f.Content, "UnrelatedEdit") {
						t.Fatalf("unrelated edit leaked via %s", f.Path)
					}
				}
				if got := planFiles(s.Files)[ch+"proposal.md"].Content; got != "# Proposal X\n" {
					t.Fatalf("proposal content = %q", got)
				}
			},
		},
		{
			name: "committed change on a clean tree",
			setup: func(t *testing.T, dir string) {
				write(t, dir, ch+"proposal.md", "# P\n")
				git(t, dir, "add", "-A")
				git(t, dir, "commit", "-qm", "change")
			},
			files: []string{ch + "proposal.md"},
		},
		{
			name: "modified capability gets its main spec",
			setup: func(t *testing.T, dir string) {
				write(t, dir, "openspec/specs/review-companion/spec.md", "# Main RC\n")
				write(t, dir, "openspec/specs/other/spec.md", "# Other\n")
				write(t, dir, ch+"specs/review-companion/spec.md", "# Delta RC\n")
			},
			files:     []string{ch + "specs/review-companion/spec.md"},
			mainSpecs: []string{"openspec/specs/review-companion/spec.md"},
			check: func(t *testing.T, s PlanScope) {
				if s.MainSpecs[0].Content != "# Main RC\n" {
					t.Fatalf("main spec = %+v", s.MainSpecs[0])
				}
			},
		},
		{
			name: "new capability has no main spec",
			setup: func(t *testing.T, dir string) {
				write(t, dir, ch+"specs/new-cap/spec.md", "# New\n")
			},
			files: []string{ch + "specs/new-cap/spec.md"},
		},
		{
			name: "symlink is listed, not followed",
			setup: func(t *testing.T, dir string) {
				write(t, dir, ch+"proposal.md", "# P\n")
				outside := filepath.Join(t.TempDir(), "secret")
				write(t, filepath.Dir(outside), "secret", "PRIVATE KEY\n")
				if err := os.Symlink(outside, filepath.Join(dir, ch+"design.md")); err != nil {
					t.Fatal(err)
				}
			},
			files: []string{ch + "design.md", ch + "proposal.md"},
			check: func(t *testing.T, s PlanScope) {
				l := planFiles(s.Files)[ch+"design.md"]
				if l.Content != "" || !strings.Contains(l.Omitted, "not followed") {
					t.Fatalf("symlink entry = %+v", l)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t)
			tt.setup(t, dir)
			s, err := CollectPlan(context.Background(), filepath.Join(dir, "openspec"), "add-x")
			if err != nil {
				t.Fatal(err)
			}
			if s.Root != dir || s.Change != "add-x" {
				t.Fatalf("root %q change %q", s.Root, s.Change)
			}
			if got := planPaths(s.Files); strings.Join(got, ",") != strings.Join(tt.files, ",") {
				t.Fatalf("files = %q, want %q", got, tt.files)
			}
			if got := planPaths(s.MainSpecs); strings.Join(got, ",") != strings.Join(tt.mainSpecs, ",") {
				t.Fatalf("main specs = %q, want %q", got, tt.mainSpecs)
			}
			if tt.check != nil {
				tt.check(t, s)
			}
		})
	}
}

func TestCollectPlanEmptyChange(t *testing.T) {
	dir := gitRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "openspec/changes/add-x/specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := CollectPlan(context.Background(), dir, "add-x")
	if !errors.Is(err, ErrNothingToReview) {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectPlanMissingChange(t *testing.T) {
	dir := gitRepo(t)
	_, err := CollectPlan(context.Background(), dir, "add-y")
	if err == nil || errors.Is(err, ErrNothingToReview) || !strings.Contains(err.Error(), "add-y") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectPlanRejectsPathLikeName(t *testing.T) {
	dir := gitRepo(t)
	write(t, dir, "openspec/secrets/x.md", "s\n")
	for _, name := range []string{"", ".", "..", "../secrets", "a/b"} {
		if _, err := CollectPlan(context.Background(), dir, name); err == nil {
			t.Errorf("%q: want error", name)
		}
	}
}

// outsideDir makes a directory outside the repo holding a change-like tree
// whose files all carry the OUTSIDE marker.
func outsideDir(t *testing.T) string {
	t.Helper()
	d, _ := filepath.EvalSymlinks(t.TempDir())
	write(t, d, "add-x/proposal.md", "OUTSIDE proposal\n")
	write(t, d, "add-x/specs/rc/spec.md", "OUTSIDE delta\n")
	write(t, d, "rc/spec.md", "OUTSIDE main spec\n")
	write(t, d, "spec.md", "OUTSIDE main spec\n")
	return d
}

func assertNoOutside(t *testing.T, s PlanScope) {
	t.Helper()
	for _, f := range append(append([]UntrackedFile{}, s.Files...), s.MainSpecs...) {
		if strings.Contains(f.Content, "OUTSIDE") {
			t.Fatalf("outside content leaked via %s", f.Path)
		}
	}
}

func TestCollectPlanRefusesSymlinkedChangeParents(t *testing.T) {
	cases := map[string]func(t *testing.T, dir, out string){
		"symlinked openspec/changes": func(t *testing.T, dir, out string) {
			os.MkdirAll(filepath.Join(dir, "openspec"), 0o755)
			if err := os.Symlink(out, filepath.Join(dir, "openspec/changes")); err != nil {
				t.Fatal(err)
			}
		},
		"symlinked change dir": func(t *testing.T, dir, out string) {
			os.MkdirAll(filepath.Join(dir, "openspec/changes"), 0o755)
			if err := os.Symlink(filepath.Join(out, "add-x"), filepath.Join(dir, "openspec/changes/add-x")); err != nil {
				t.Fatal(err)
			}
		},
		"symlinked openspec": func(t *testing.T, dir, out string) {
			write(t, out, "os/changes/add-x/proposal.md", "OUTSIDE\n")
			if err := os.Symlink(filepath.Join(out, "os"), filepath.Join(dir, "openspec")); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := gitRepo(t)
			setup(t, dir, outsideDir(t))
			s, err := CollectPlan(context.Background(), dir, "add-x")
			if err == nil {
				assertNoOutside(t, s)
				t.Fatal("want an error for a symlinked change path")
			}
			if !strings.Contains(err.Error(), "symlink") || !strings.Contains(err.Error(), "openspec") {
				t.Fatalf("err = %v, want it to name the symlinked path", err)
			}
		})
	}
}

func TestCollectPlanListsSymlinkedMainSpecDir(t *testing.T) {
	for name, link := range map[string]string{
		"symlinked openspec/specs/<cap>": "openspec/specs/rc",
		"symlinked openspec/specs":       "openspec/specs",
	} {
		t.Run(name, func(t *testing.T) {
			dir := gitRepo(t)
			out := outsideDir(t)
			write(t, dir, "openspec/changes/add-x/specs/rc/spec.md", "# Delta\n")
			os.MkdirAll(filepath.Dir(filepath.Join(dir, link)), 0o755)
			target := filepath.Join(out, "rc")
			if link == "openspec/specs" {
				target = out
			}
			if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
				t.Fatal(err)
			}
			s, err := CollectPlan(context.Background(), dir, "add-x")
			if err != nil {
				t.Fatal(err)
			}
			assertNoOutside(t, s)
			if len(s.MainSpecs) != 1 || s.MainSpecs[0].Path != "openspec/specs/rc/spec.md" || !strings.Contains(s.MainSpecs[0].Omitted, "not followed") {
				t.Fatalf("main specs = %+v", s.MainSpecs)
			}
		})
	}
}
