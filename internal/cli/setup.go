package cli

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shurastik/agterm-kit/internal/setup"
)

const setupUsage = `Usage: agk setup [--skills-dir DIR] [--dry-run]

Links the agk-* agent skills into ~/.claude/skills and ~/.codex/skills, adds .agk/ to the
global git ignore file, and checks git, codex, claude, openspec and agtermctl. Run it after
every install or upgrade; it is idempotent and never touches the current repository.

The skills come from --skills-dir, else $AGK_SKILLS_DIR, else the Homebrew install.
`

// Setup implements `agk setup`.
func (e *Env) Setup(args []string) int {
	fs := flag.NewFlagSet("agk setup", flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	fs.Usage = func() { fmt.Fprint(e.Stderr, setupUsage) }
	skillsFlag := fs.String("skills-dir", "", "directory holding the agk-* skills")
	dryRun := fs.Bool("dry-run", false, "print the actions without changing anything")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitError
	}
	if fs.NArg() != 0 {
		return e.errorf("unexpected arguments %q", fs.Args())
	}
	flagDir := *skillsFlag
	if flagDir != "" && !filepath.IsAbs(flagDir) {
		flagDir = filepath.Join(e.Cwd, flagDir)
	}
	exe, err := e.Executable()
	if err != nil {
		return e.errorf("locate agk: %v", err)
	}
	dir, err := setup.ResolveSkillsDir(flagDir, e.Getenv("AGK_SKILLS_DIR"), exe)
	if err != nil {
		return e.errorf("%v", err)
	}
	acts, err := setup.Plan(setup.Config{
		Home: e.Home, SkillsDir: dir, Getenv: e.Getenv,
		GitConfig: e.GitConfig, LookPath: e.LookPath, IsExec: e.IsExec,
	})
	if err != nil {
		return e.errorf("%v", err)
	}
	var applyErr error
	if !*dryRun {
		applyErr = setup.Apply(acts)
	}
	fmt.Fprintf(e.Stdout, "skills   %s\n", e.tilde(dir))
	code := ExitOK
	for _, a := range acts {
		fmt.Fprintln(e.Stdout, e.describe(a))
		if a.Failed() {
			code = ExitError
		}
	}
	if *dryRun {
		fmt.Fprintln(e.Stdout, "dry run: nothing changed")
	}
	if applyErr != nil {
		fmt.Fprintf(e.Stderr, "agk: %v\n", applyErr)
		code = ExitError
	}
	return code
}

func (e *Env) describe(a setup.Action) string {
	line := fmt.Sprintf("%-8s %s", a.Kind, e.tilde(a.Path))
	switch {
	case a.Kind == setup.KindFound:
		line += " " + e.tilde(a.Target)
	case a.Target != "":
		line += " -> " + e.tilde(a.Target)
	}
	if a.Kind == setup.KindMissing {
		if a.Optional {
			line += " (optional)"
		} else {
			line += " (required)"
		}
	}
	if a.Detail != "" {
		line += " (" + a.Detail + ")"
	}
	return line
}

func (e *Env) tilde(p string) string {
	if e.Home != "" && (p == e.Home || strings.HasPrefix(p, e.Home+"/")) {
		return "~" + p[len(e.Home):]
	}
	return p
}
