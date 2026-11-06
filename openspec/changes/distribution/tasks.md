# Tasks

## 1. Version and build floor

- [x] 1.1 Add `internal/buildinfo` with `Version = "dev"` and `agk version` in `internal/cli` (usage text included); verify with a CLI test that prints `dev` by default and the stamped value when the test sets `buildinfo.Version`, and with `go build -ldflags "-X github.com/shurastik/agterm-kit/internal/buildinfo.Version=0.0.1" -o .bin/agk ./cmd/agk && .bin/agk version` printing `0.0.1`
- [x] 1.2 Lower the `go` directive per design D10: try `GOTOOLCHAIN=go1.26.0 go test ./...`, set `go 1.26.0` if it passes, else `go 1.27.0`; verify `go build ./... && go test ./...` pass with the local toolchain and the AGENTS.md runtime note matches

## 2. Skill relocation

- [x] 2.1 `git mv skill/agterm-codex-review skills/agk-review`, set `name: agk-review` and update self-references per design D9; verify `skills/agk-review/SKILL.md` front matter parses (name, description present) and `grep -rn "agterm-codex-review\|skill/" --exclude-dir=.git --exclude-dir=archive .` finds only intended legacy mentions (the setup cleanup code and its tests)

## 3. Setup planning (`internal/setup`)

- [x] 3.1 Skills directory resolution per design D4 (flag, `$AGK_SKILLS_DIR`, Cellar → `opt` path, `exe/../share/agk/skills`, error naming `--skills-dir`); verify with table tests over temp dirs, including a fake Cellar layout `<tmp>/Cellar/agk/0.3.0/bin/agk` resolving to `<tmp>/opt/agk/share/agk/skills`
- [x] 3.2 Link planning for Claude (`$CLAUDE_CONFIG_DIR` or `~/.claude`) and Codex (`$CODEX_HOME` or `~/.codex`) per design D5/D6: `create`, `ok`, `repoint`, `conflict` for regular file/dir, `skip` for a missing agent config dir, `remove` for dangling `agk-*` links and the legacy `agterm-codex-review` link, unrelated entries untouched; verify with tests against a temp `HOME` covering every scenario of the "Skill linking", "Setup is idempotent" and "Stale and legacy links are removed" requirements
- [x] 3.3 Excludes-file planning per design D7 (git config path via injected func, XDG fallback, default; presence check for `.agk`, `.agk/`, `/.agk`, `/.agk/`); verify with tests for each path source and each accepted spelling
- [x] 3.4 Dependency checks per design D8 (`git`, `codex`, `claude`, `openspec` required; `agtermctl` optional through `agterm.ResolveBin`); verify with tests using a fake `lookPath`

## 4. Setup apply and command

- [x] 4.1 `setup.Apply`: symlink via temp name + rename, remove only symlinks, append to the resolved excludes target with `fsx.ReplaceFile` (missing trailing newline handled, file mode kept, parent dir created); verify with tests: links point at absolute skill paths, a symlinked excludes file stays a symlink and its target gains one `.agk/` line, a second plan after apply is all `ok`
- [x] 4.2 `agk setup [--skills-dir DIR] [--dry-run]` in `internal/cli` with the injected home/env/lookPath/gitConfig on `Env`; one output line per action; exit 1 on any conflict or missing required tool, after applying the rest; verify with CLI tests for dry run (filesystem unchanged), conflict exit 1 with other links created, missing `openspec` exit 1, missing `agtermctl` exit 0, and that nothing is written under the working directory
- [x] 4.3 Update `AGENTS.md` (install via brew + `agk setup`, dev loop with `--skills-dir ./skills`, layout table: `skills/`, `internal/setup/`, `internal/buildinfo/`, `packaging/`, `scripts/`) and the usage text in `agk --help`; verify the documented commands match `agk --help` and `agk setup --help`

## 5. Packaging and release

- [x] 5.1 Add `packaging/homebrew/agk.rb.tmpl` per design D1 (tag tarball URL, sha256, `go build` with the version ldflag, `pkgshare.install "skills"`, caveat to run `agk setup`, `test do` running `agk version`); verify by rendering it for the current tree and running `brew audit --strict --formula` on the rendered file from a local tap
- [x] 5.2 Add `scripts/release.sh VERSION` per design D2 (clean `main` in sync with origin, quality gates, tag + push, tarball sha256, render into `$AGK_TAP_DIR`, commit + push the tap); verify with `bash -n` and `shellcheck`, and a run with `DRY_RUN=1` that prints the rendered formula without tagging or pushing
- [ ] 5.3 Publish prerequisites, each only after the owner says go: scan history for secrets and list remote refs (design Risks), report findings; make `shurastik/agterm-kit` public; create the public `shurastik/homebrew-tap` repo with a README; verify with `gh repo view --json visibility` for both

## 6. Integration

- [x] 6.1 Quality gates: `go build ./... && go vet ./... && go test ./...` pass
- [ ] 6.2 First release `0.1.0` with `scripts/release.sh`, then on this machine: `brew install shurastik/tap/agk`, `agk version` prints `0.1.0`, `agk setup --dry-run` then `agk setup`; verify `~/.claude/skills/agk-review` and `~/.codex/skills/agk-review` link into `/opt/homebrew/opt/agk/share/agk/skills/`, the legacy link is gone, `~/.config/git/ignore` has one `.agk/` line, and a second `agk setup` reports everything `ok`
- [ ] 6.3 Remove `~/go/bin/agk`, start a fresh Claude Code session and a fresh Codex session and verify each lists the `agk-review` skill; run `agk review start` in a scratch repo to confirm the brew binary works end to end

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Archive the change and sync the `installation` spec.
