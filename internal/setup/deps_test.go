package setup

import (
	"errors"
	"testing"

	"github.com/shurastik/agterm-kit/internal/agterm"
)

func depsCfg(onPath map[string]bool, vars map[string]string, execs map[string]bool) Config {
	return Config{
		Getenv: func(k string) string { return vars[k] },
		LookPath: func(n string) (string, error) {
			if onPath[n] {
				return "/bin/" + n, nil
			}
			return "", errors.New("not found")
		},
		IsExec: func(p string) bool { return execs[p] },
	}
}

func byPath(acts []Action) map[string]Action {
	m := map[string]Action{}
	for _, a := range acts {
		m[a.Path] = a
	}
	return m
}

func TestDepsAllPresent(t *testing.T) {
	onPath := map[string]bool{"git": true, "codex": true, "claude": true, "openspec": true, "agtermctl": true}
	acts := planDeps(depsCfg(onPath, nil, nil))
	m := byPath(acts)
	for _, n := range []string{"git", "codex", "claude", "openspec", "agtermctl"} {
		if a := m[n]; a.Kind != KindFound || a.Target != "/bin/"+n {
			t.Errorf("%s: %+v", n, a)
		}
	}
	for _, a := range acts {
		if a.Failed() {
			t.Errorf("failed action %+v", a)
		}
	}
}

func TestDepsMissingRequired(t *testing.T) {
	onPath := map[string]bool{"git": true, "codex": true, "claude": true, "agtermctl": true}
	a := byPath(planDeps(depsCfg(onPath, nil, nil)))["openspec"]
	if a.Kind != KindMissing || a.Optional || !a.Failed() {
		t.Fatalf("%+v, want required missing", a)
	}
}

func TestDepsMissingAgtermIsOptional(t *testing.T) {
	onPath := map[string]bool{"git": true, "codex": true, "claude": true, "openspec": true}
	a := byPath(planDeps(depsCfg(onPath, nil, nil)))["agtermctl"]
	if a.Kind != KindMissing || !a.Optional || a.Failed() {
		t.Fatalf("%+v, want optional missing", a)
	}
}

func TestDepsAgtermctlResolvedLikeHooks(t *testing.T) {
	a := byPath(planDeps(depsCfg(nil, nil, map[string]bool{agterm.BundledBin: true})))["agtermctl"]
	if a.Kind != KindFound || a.Target != agterm.BundledBin {
		t.Fatalf("bundle: %+v", a)
	}
	a = byPath(planDeps(depsCfg(nil, map[string]string{"AGTERMCTL": "/opt/x/agtermctl"}, map[string]bool{"/opt/x/agtermctl": true})))["agtermctl"]
	if a.Kind != KindFound || a.Target != "/opt/x/agtermctl" {
		t.Fatalf("env: %+v", a)
	}
	a = byPath(planDeps(depsCfg(nil, map[string]string{"AGTERMCTL": "/opt/x/agtermctl"}, nil)))["agtermctl"]
	if a.Kind != KindMissing {
		t.Fatalf("env not executable: %+v", a)
	}
}
