package setup

import (
	"strings"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

// Required tools must be on PATH for the agk workflow.
var Required = []string{"git", "codex", "claude", "openspec"}

func planDeps(c Config) []Action {
	var acts []Action
	for _, name := range Required {
		acts = append(acts, lookup(c, name, name, false))
	}
	bin := agterm.ResolveBin(c.Getenv, c.IsExec)
	if strings.Contains(bin, "/") {
		if c.IsExec(bin) {
			acts = append(acts, Action{Kind: KindFound, Path: "agtermctl", Target: bin})
		} else {
			acts = append(acts, Action{Kind: KindMissing, Path: "agtermctl", Optional: true, Detail: bin + " is not executable"})
		}
		return acts
	}
	return append(acts, lookup(c, "agtermctl", bin, true))
}

func lookup(c Config, name, bin string, optional bool) Action {
	if p, err := c.LookPath(bin); err == nil {
		return Action{Kind: KindFound, Path: name, Target: p}
	}
	return Action{Kind: KindMissing, Path: name, Optional: optional}
}
