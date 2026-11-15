package cli

import (
	"context"
	"github.com/shurastik/agterm-kit/internal/agterm"
	"strings"

	"github.com/shurastik/agterm-kit/internal/worktree"
)

// wtCall is one recorded call of the fake worktree.
type wtCall struct {
	Op   string
	Args []string
}

func (c wtCall) String() string { return strings.TrimSpace(c.Op + " " + strings.Join(c.Args, " ")) }

// fakeWorktree records every call and returns scripted results. The zero
// results describe a quiet main checkout: nothing at .worktrees/<change>, no
// branch of that name, base `main` with an upstream, a plan to commit.
type fakeWorktree struct {
	calls []wtCall
	errs  map[string]error // by operation name

	loc         worktree.Location
	branch      string // CurrentBranch of any directory
	upstream    string
	hasUpstream bool
	entry       worktree.Entry // Lookup; Path is filled in when empty
	branchOK    bool           // BranchExists
	committed   bool           // CommitPaths
	addPath     string         // Add; defaults to .worktrees/<change> under the main checkout
	agkBase     string
	hasAgkBase  bool
	status      []string
	archived    bool
	ancestors   map[string]bool     // "ancestor>descendant"; absent means false
	heads       map[string][]string // by ref; each call takes the first, the last repeats
	rebaseErr   error
	onRebase    func()
	onMerge     func()
	onRemove    func()
}

func newFakeWorktree(main string) *fakeWorktree {
	return &fakeWorktree{
		errs:        map[string]error{},
		loc:         worktree.Location{Main: main, Root: main},
		branch:      "main",
		upstream:    "origin/main",
		hasUpstream: true,
		committed:   true,
		ancestors:   map[string]bool{},
		heads:       map[string][]string{},
	}
}

// ops lists the called operation names in order.
func (f *fakeWorktree) ops() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, c.Op)
	}
	return out
}

func (f *fakeWorktree) called(op string) bool {
	for _, c := range f.calls {
		if c.Op == op {
			return true
		}
	}
	return false
}

func (f *fakeWorktree) rec(op string, args ...string) error {
	f.calls = append(f.calls, wtCall{op, args})
	return f.errs[op]
}

func (f *fakeWorktree) Locate(_ context.Context, dir string) (worktree.Location, error) {
	return f.loc, f.rec("Locate", dir)
}

func (f *fakeWorktree) CurrentBranch(_ context.Context, dir string) (string, error) {
	err := f.rec("CurrentBranch", dir)
	if err != nil {
		return "", err
	}
	return f.branch, nil
}

func (f *fakeWorktree) Upstream(_ context.Context, dir, branch string) (string, bool, error) {
	if err := f.rec("Upstream", dir, branch); err != nil {
		return "", false, err
	}
	return f.upstream, f.hasUpstream, nil
}

func (f *fakeWorktree) Lookup(_ context.Context, main, change string) (worktree.Entry, error) {
	err := f.rec("Lookup", main, change)
	e := f.entry
	if e.Path == "" {
		e.Path = worktree.WorktreePath(main, change)
	}
	return e, err
}

func (f *fakeWorktree) BranchExists(_ context.Context, dir, branch string) (bool, error) {
	return f.branchOK, f.rec("BranchExists", dir, branch)
}

func (f *fakeWorktree) PullFFOnly(_ context.Context, dir string) error {
	return f.rec("PullFFOnly", dir)
}

func (f *fakeWorktree) CommitPaths(_ context.Context, root, message string, paths ...string) (bool, error) {
	err := f.rec("CommitPaths", append([]string{root, message}, paths...)...)
	return f.committed, err
}

func (f *fakeWorktree) Add(_ context.Context, main, change, base string) (string, error) {
	if err := f.rec("Add", main, change, base); err != nil {
		return "", err
	}
	if f.addPath != "" {
		return f.addPath, nil
	}
	return worktree.WorktreePath(main, change), nil
}

func (f *fakeWorktree) AgkBase(_ context.Context, dir, branch string) (string, bool, error) {
	err := f.rec("AgkBase", dir, branch)
	return f.agkBase, f.hasAgkBase, err
}

func (f *fakeWorktree) Status(_ context.Context, dir string) ([]string, error) {
	return f.status, f.rec("Status", dir)
}

func (f *fakeWorktree) ArchivedOnBranch(_ context.Context, dir, change string) (bool, error) {
	return f.archived, f.rec("ArchivedOnBranch", dir, change)
}

func (f *fakeWorktree) IsAncestor(_ context.Context, dir, ancestor, descendant string) (bool, error) {
	err := f.rec("IsAncestor", ancestor, descendant)
	return f.ancestors[ancestor+">"+descendant], err
}

func (f *fakeWorktree) Head(_ context.Context, dir, ref string) (string, error) {
	err := f.rec("Head", ref)
	h := f.heads[ref]
	switch {
	case len(h) == 0:
		return "sha-" + ref, err
	case len(h) == 1:
		return h[0], err
	}
	f.heads[ref] = h[1:]
	return h[0], err
}

func (f *fakeWorktree) Rebase(_ context.Context, dir, onto string) error {
	if err := f.rec("Rebase", dir, onto); err != nil {
		return err
	}
	if f.onRebase != nil {
		f.onRebase()
	}
	return f.rebaseErr
}

func (f *fakeWorktree) MergeFFOnly(_ context.Context, dir, branch string) error {
	if err := f.rec("MergeFFOnly", dir, branch); err != nil {
		return err
	}
	if f.onMerge != nil {
		f.onMerge()
	}
	return nil
}

func (f *fakeWorktree) Remove(_ context.Context, dir, path string) error {
	if f.onRemove != nil {
		f.onRemove()
	}
	return f.rec("Remove", dir, path)
}

func (f *fakeWorktree) DeleteBranch(_ context.Context, dir, branch string) error {
	return f.rec("DeleteBranch", dir, branch)
}

func agtermGone() error { return agterm.ErrGone }
