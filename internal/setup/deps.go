package setup

import (
	"strings"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

// Required tools must be on PATH for the agk workflow.
var Required = []string{"git", "codex", "claude", "openspec"}

// Optional tools on PATH: the workflow skips their step when they are missing
// (revdiff: the user's file review in agk-propose).
var Optional = []string{"revdiff"}

func planDeps(c Config) []Action {
	var acts []Action
	for _, name := range Required {
		acts = append(acts, lookup(c, name, name, false))
	}
	acts = append(acts, agtermctl(c))
	for _, name := range Optional {
		acts = append(acts, lookup(c, name, name, true))
	}
	return acts
}

// agtermctl resolves agtermctl the way agterm's hooks do.
func agtermctl(c Config) Action {
	bin := agterm.ResolveBin(c.Getenv, c.IsExec)
	if strings.Contains(bin, "/") {
		if c.IsExec(bin) {
			return Action{Kind: KindFound, Path: "agtermctl", Target: bin}
		}
		return Action{Kind: KindMissing, Path: "agtermctl", Optional: true, Detail: bin + " is not executable"}
	}
	return lookup(c, "agtermctl", bin, true)
}

func lookup(c Config, name, bin string, optional bool) Action {
	if p, err := c.LookPath(bin); err == nil {
		return Action{Kind: KindFound, Path: name, Target: p}
	}
	return Action{Kind: KindMissing, Path: name, Optional: optional}
}
