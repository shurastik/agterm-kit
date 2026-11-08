# Tasks

## 1. Claude reviewer spike

- [x] 1.1 Spike the claude TUI path (design D4) in a scratch git repo inside agterm. Run `claude --restricted --strict-mcp-config --tools Read,Grep,Glob,Bash,Write --permission-mode dontAsk --allowedTools "Bash(git diff:*),Bash(git status:*),Edit(//<abs scratch>/out/**)" --add-dir <scratch>/out --settings <file with UserPromptSubmit/Stop/Notification hooks that append to a log>` and ask it to (a) run the exact git commands the final claude prompt prescribes (`git diff <mergebase>`, `git status`, the plain read-rest command), (b) write `out/1.json`, (c) write a repo file, (d) run `go version`. Verify: the TUI starts; (a) and (b) succeed without a prompt; (c) and (d) are denied without a prompt; the hook log shows the start and stop of the turn. Record the result in `design.md` D4. If any check fails, stop and report `blocked` with artifact `design`.

## 2. Shared worker plumbing

- [x] 2.1 Move `handshake`, `markStarted`/`roundStarted` and `waitPolling`/`waitSliced` from `internal/cli/apply.go` into `internal/cli/worker.go`. Give `waitSliced` the path of the file that ends the wait instead of the report of round k. Make `waitPolling` check the remaining budget before the file, so an existing file never returns with a spent budget unnoticed. Verify that `go test ./internal/cli/...` passes with the `apply_test.go` cases unchanged, plus a `worker_test.go` case: the file exists from the start and the budget runs out, so the call returns the budget as spent.
- [x] 2.2 Extract `typeWhenIdle(ctx, companion, line, timeout)` from `ApplySend`: wait for `completed|blocked|idle`, refuse on `blocked` / gone / timeout with the worker name in the message, type `line\r`, then run the 30 s `active` handshake (warn on stderr when it is missing). Switch `ApplySend` to it. Verify that `apply_test.go` send cases (busy, permission, gone, unconfirmed start) pass unchanged, plus a `worker_test.go` case for each refusal on a fake `Ctl`.

## 3. Run state, schema and prompts

- [x] 3.1 Add `Interactive bool` (`interactive`, omitempty) to `reviewState`, and an `OutDir`/`schema.json` helper that creates `<run>/out/` and writes the embedded schema. Verify in `review_start_test.go` that a `worker.json` without the field loads as non-interactive and that the helper writes `schema.json` byte-equal to `assets.ReviewSchema`.
- [x] 3.2 Add an output block to `assets/review-prompt.md`, `assets/plan-review-prompt.md` and `assets/rereview-prompt.md`. Interactive: absolute `schema.json`, `out/<k>.json` written as the last step, the repository root, paths relative to the root, and the git form per harness (codex `git -C <root>`, claude plain `git`). Inline: today's output contract, unchanged. Add an `Output` parameter (paths plus git form) to `review.Prompt`, `review.PlanPrompt` and `review.Rereview`, and a root argument to `ReadRestCommand` that renders `git -C <root> diff …` for codex. Verify in `internal/review/prompt_test.go`: an interactive codex prompt names all three paths and its read-rest command carries `-C <root>`; an interactive claude prompt has no `git -C` anywhere; inline output is byte-identical to the current golden outputs. Add a `scope_test.go` case running the codex read-rest command from a subdirectory of a temp repo, which reproduces the repository diff.
- [x] 3.3 Add `assets/review-fixup-prompt.md`: the output was invalid, here is the validator error as data, rewrite `out/<k>.json` to match `schema.json`. Embed it and add `review.FixupPrompt(k, out, schema, err)`. Verify with an assets parse test and a prompt test showing the error inside a data block.

## 4. Harness argv

- [x] 4.1 Add `InteractiveReviewer(w, root, out string, f ReviewerFiles, session, kickoff string) []string` to `harness.Driver`. Codex returns `codex [tuning] --sandbox workspace-write --ask-for-approval never -C <out> -c sandbox_workspace_write.writable_roots=[] -c sandbox_workspace_write.exclude_tmpdir_env_var=true -c sandbox_workspace_write.exclude_slash_tmp=true <kickoff>`, with the mandatory flags after tuning. Verify with exact-argv tests in `internal/harness/codex_test.go`, with and without model, effort and `extra_args`, and that every mandatory flag comes after the last tuning argument.
- [x] 4.2 Claude `InteractiveReviewer`: `claude [tuning] --restricted --strict-mcp-config --tools Read,Grep,Glob,Bash,Write --permission-mode dontAsk --allowedTools <ReviewerTools()>,Edit(//<abs out>/**) --add-dir <out> --settings <f.Settings> --session-id <session> <kickoff>`. Verify with exact-argv tests in `internal/harness/claude_test.go`, including the `//` absolute form of the `Edit` rule (a `Write(…)` allow rule is not matched by file permission checks) and the mandatory flags after tuning.
- [x] 4.2a Refuse widening reviewer `extra_args` (design D4a) per harness, in `agk review start` before the run dir is created, for inline and interactive runs. Verify with table tests: each listed flag and its `=value` form, plus `-c sandbox_mode=…`/`--config approval_policy=…` for codex, exits 1 naming it with no run dir and no `NewCompanion`; harmless tuning such as `-c model_verbosity="low"` passes; the applier's `extra_args` are unaffected.
- [x] 4.3 Generate `<run>/reviewer-settings.json` for a claude reviewer. Its hooks are `UserPromptSubmit` → `'<agk>' review hook active`, `Stop` → `… completed` and `Notification` (permission matcher) → `… blocked`, with `<agk>` quoted via `shellQuote` from the resolved executable. Verify with a unit test that parses the JSON and checks the hook commands, and that a path with a backslash is refused.
- [x] 4.4 Add the hidden `agk review hook <active|completed|blocked>` command. It sets the agent status of `$AGTERM_SESSION_ID` through `Ctl.Status`, exits 0 without calling agterm when `AGTERM_SESSION_ID` is unset, and exits 1 on an unknown state. Verify in `review_hook_test.go` with the fake `Ctl` (target and state recorded, `--socket` passed through the existing `Ctl`) and in the `main_test.go` dispatch.

## 5. Wait decision

- [x] 5.1 Add `review.Decide(k, output []byte, exists bool, status string, waitErr error, fixups int) Action`. Inputs also carry `fixupStarted bool` (whether the latest fix-up has started, design D6). Rules: a valid output gives `record completed` first, whatever the gate; while a typed fix-up has not started, `blocked` gives `needs-human: permission`, gone gives `cancelled`, timeout gives `running`, and everything else (old `completed`/`idle`, an invalid file changed or not) gives `keep waiting`; otherwise a valid output gives `record completed` at any status; an invalid output while the turn is running gives `keep waiting`; an invalid output after the turn gives `type fix-up` while `fixups < 2` and `record blocked: invalid review output` after that; no output after the turn (`completed`/`idle`) gives `needs-human: no-output`; `blocked` without output gives `needs-human: permission`; gone gives `cancelled`; timeout gives `running`. Verify with a table test in `internal/review/decide_test.go` covering every row, including a half-written file mid-turn.
- [x] 5.2 Interactive `ReviewWait`:
  - `result.json` first;
  - otherwise the round-start gate, then `waitSliced` on `out/<k>.json` for `completed|blocked|idle`;
  - act on `Decide`:
    - record: write `review.json` and `result.json`, then the info line only (`Ctl.Info`, never `Ctl.Status`);
    - fix-up: write `fixup-<n>.md`, increment `rounds/<k>/fixups`, `typeWhenIdle`, write `fixup-<n>.started` when its handshake sees `active`, loop;
    - fix-up start gate: while the latest fix-up has no `.started` marker, wait in slices for `active` (then write the marker), `blocked`, gone or a valid `out/<k>.json`, under the same deadline; a changed invalid file never opens the gate;
    - failure: copy the raw output to `rounds/<k>/output.json`.

  Verify in `review_wait_test.go` with a fake `Ctl` and files on disk: valid output mid-turn completes; invalid output gives one fix-up typed and then a valid output completes; three invalid outputs block and keep the raw output; an invalid file that outlasts `--timeout` while the turn runs gives `running` on time; no output gives `needs-human`/`no-output` with no `result.json`; `blocked` gives `needs-human`/`permission`; a second wait after `needs-human` completes; an unconfirmed fix-up handshake followed by a new wait with the old `completed` and unchanged file types no second fix-up and gives `running` on timeout; after an unconfirmed fix-up a changed invalid file with the old `completed` types nothing and does not block, and becoming valid completes the round; an unconfirmed fix-up with `blocked` gives `needs-human`/`permission` at once with no `result.json`; gone gives `cancelled`; timeout gives `running`; the info line holds only counts and fixed strings; no `Ctl.Status` call is made in any of these cases.
- [x] 5.3 Older runs (companion present, `interactive` unset): `wait` prints `result.json` when present and otherwise exits 1 with the "started by an older agk" message. Verify in `review_wait_test.go`, both cases, with no `Ctl` wait call.

## 6. Start and send

- [x] 6.1 Interactive `ReviewStart` in agterm:
  - record `interactive: true` and create `out/` + `schema.json`;
  - render round 1's prompt with the interactive output block;
  - for claude, write `reviewer-settings.json` and a session id;
  - start the companion with `envCommand(driver.InteractiveReviewer(...))` under the name `<harness>-review #N`;
  - `handshake`; on success `markStarted` and the info line `<h> #N r1: reviewing` (info only, no status);
  - print `{"companion","dir","started"}`.

  Inline stays the same. Verify in `review_start_test.go`: exact companion command for codex and claude, `started` true/false (false keeps the companion open and warns with screen lines), `--change` and `--base` both go through the same path, inline output unchanged.
- [x] 6.2 Interactive `ReviewSend`:
  - add `--timeout` (default 600);
  - refuse without `result.json` for round k, and refuse older runs;
  - render the re-review prompt for k+1 with the interactive output block;
  - `initRound`, then `typeWhenIdle` of the kickoff line; on refusal roll back the round (`SetRound`, remove the dir);
  - set the info line `r<k+1>: reviewing`;
  - print `{"companion","dir","round"}`.

  Verify in `review_send_test.go`: typed line and prompt content (note, `out/2.json`); busy, permission and gone record no round; a round recorded mid-turn (status still `active`) waits for the turn and refuses as busy on a short timeout; a round failed by validation (status `idle`) sends normally; an older run is refused; a plan run on a clean tree works; inline is unchanged.

## 7. Removals and docs

- [x] 7.1 Remove `agk review run`, `ReviewRun`, `nextRound`, `paneFile` and their tests and dispatch. Drop `MsgRead`/`MsgSend` from `agterm.Ctl`, the CLI and the fake if nothing else uses them. Verify that `go build ./...`, `go vet ./...` and `go test ./...` pass and `grep -rn "review run\|MsgRead\|MsgSend" internal cmd` finds nothing.
- [x] 7.2 Update `AGENTS.md`:
  - reviewer description: interactive in agterm, headless inline;
  - usage: `review start` prints `started`, `review wait` statuses include `needs-human`, `review send --timeout`;
  - conventions: the "reviewer child runs without `AGTERM_*`" bullet becomes the interactive reviewer rule, plus claude's agk-owned status hooks.

  Verify by reading each documented command against `.bin/agk review <sub> -h`.
- [x] 7.3 Update `skills/agk-review/SKILL.md`: the `started` field; a `needs-human` branch (`no-output`: show the user and let them answer in the reviewer companion, then wait again; `permission`: ask the user to answer it in the companion); `send --timeout` and its busy/permission refusals. Update `skills/agk-propose/SKILL.md` where it branches on wait statuses. Verify that `scripts/eval-skills.sh` still passes the `agk-propose` suite. If the stand-in `agk` needs `started`, add it to `evals/_fixtures/bin/agk` and re-run `evals/_fixtures/test.sh`.

## 8. Integration

- [x] 8.1 Run `go test ./...`, `go vet ./...` and `go build -o .bin/agk ./cmd/agk`. Then, inside agterm, run with the dev build a diff review with `--reviewer codex` and one with `--reviewer claude` on a small scratch diff. Confirm that the companion shows the harness TUI, `review wait` returns `completed` with `review.json`, a `send --note` round is typed into the same session, a write attempt to the repository is denied, and `git status` of the repo is unchanged by the reviewer.

## Workflow follow-up

- Codex review of the diff until clean, then `/opsx:verify` with 0 CRITICAL.
- Archive the change and close the beads epic.
