## Context

See proposal.md for the motivation.

- **Reviewer today.** Inside agterm the reviewer companion runs `agk review run <dir>`
  (`internal/cli/review.go`, `ReviewRun`). That process calls `Driver.Review` per round:
  `codex exec --json --output-schema` or `claude -p --output-format stream-json --json-schema`.
  It renders the event stream as timestamped lines (`renderCodex`), validates the output with
  `review.Parse`, writes `review.json`/`result.json` and sets the companion's status. Between
  rounds it blocks in its agterm mailbox (`nextRound`) until `agk review send` posts `round k`.
- **Applier today.** It is already an interactive harness in its companion
  (`Driver.Interactive`). A round starts with one typed line (`kickoff`). The end of a turn comes
  from the harness's agterm status hooks. `agk apply wait` decides from the report file plus the
  status (`apply.Decide`, `waitSliced`, `roundStarted`), and `agk apply send` types the next round
  only when the turn is over and no permission prompt is open.
- **Sandbox spike (codex-cli 0.167.0, `codex sandbox`).** With `-C <run>/out`, `sandbox_mode =
  "workspace-write"` and both `sandbox_workspace_write.exclude_*` keys set, the only writable
  root is `<run>/out`. Writes to repository files, the repository root, `.git` and the sibling
  run directory fail with `Operation not permitted`. Reading the repository and `git -C <root>
  status|diff` work. `codex debug prompt-input` from the run directory still loads the root
  `AGENTS.md`.
- **Claude.** `claude --restricted` ignores user, project and local settings files. The agterm
  claude status hooks live in `~/.claude/settings.json`. `--settings` still applies under
  `--restricted`.

## Goals / Non-Goals

**Goals:**
- The reviewer in agterm behaves like the applier: the harness's own UI, typed rounds, status from
  hooks, a decision from files.
- Read-only stays a harness guarantee for both harnesses.
- One copy of the companion plumbing (handshake, sliced waits, typing when the turn is over) for
  both workers.

**Non-Goals:**
- Changing the inline (outside agterm) review path, its log or its schema enforcement.
- Changing the review prompts' review method or finding bar.
- Letting agk answer the reviewer's questions: `needs-human` goes to the main session and the user.
- Restricting the codex reviewer's network or user hooks beyond what the sandbox already does.

## Decisions

### D1. Interactive reviewer in agterm, headless only inline
`agk review start` in agterm runs `Driver.InteractiveReviewer(...)` as the companion command,
with round 1's kickoff line as its first prompt, the same shape as the applier. Outside agterm,
`runRound` and `Driver.Review` stay as they are.

*Alternative:* keep headless and improve the renderer (option B in explore). It is cheaper, but the
user still cannot step in, which is the point of this change.

### D2. Findings through `<run>/out/<k>.json`, validated by `agk review wait`
An interactive harness has no `--output-schema`. The prompt names `schema.json` (the embedded
schema, written into the run directory) and the output file, and the reviewer writes it with its
own write tool. There is no agk process in the companion any more, so `agk review wait` is the
validator:
- **valid output:** parse with `review.Parse`, then write `rounds/<k>/review.json` and
  `result.json` (`completed`) and set the info line;
- **invalid output while the turn runs:** wait;
- **invalid output after the turn:** fix-up (D6);
- **no file after the turn:** `needs-human`.

*Alternatives:*
- Codex `notify` capture of `last-assistant-message`. It keeps a pure read-only sandbox, but it
  overrides the user's own `notify`, its payload is not a documented contract, and claude has no
  equivalent.
- Scraping the screen with `agtermctl text`. Too fragile.

### D3. Codex: the run's `out/` as the only writable root
Argv:

```
codex [tuning] --sandbox workspace-write --ask-for-approval never -C <run>/out \
  -c sandbox_workspace_write.writable_roots=[] \
  -c sandbox_workspace_write.exclude_tmpdir_env_var=true \
  -c sandbox_workspace_write.exclude_slash_tmp=true \
  "<kickoff>"
```

- **Working directory.** The companion's cwd stays the repository root, but codex works from
  `-C`. `out/` is a fresh directory, so the reviewer cannot touch `worker.json`, earlier prompts
  or other runs.
- **Approvals.** `-a never` means a command that needs to escape the sandbox fails instead of
  asking.
- **Git.** The prompt gives the repository root and asks for `git -C <root>`. The command that
  reads the rest of a truncated diff is rendered with `-C <root>` too, because its `-- .` pathspec
  would otherwise mean `out/`.
- **Writable roots and ordering.** A user's `config.toml` may set
  `sandbox_workspace_write.writable_roots`, for example the repository or a parent. The forced
  `writable_roots=[]` resets that. The mandatory flags come after tuning, so a later `-c` wins.
  Spike: `-c writable_roots=["<repo>"]` alone lets the sandbox write the repository; adding
  `-c writable_roots=[]` after it denies the write again.

*Alternatives:*
- cwd = repository root with `workspace-write`: the repository is writable.
- cwd = run directory: `worker.json` and the prompts are writable.
- `read-only` plus a writable root: codex has no such mode.

### D4. Claude: restricted mode, path-scoped `Write`, agk's own status hooks
Argv:

```
claude [tuning] --restricted --strict-mcp-config --tools Read,Grep,Glob,Bash,Write \
  --permission-mode dontAsk \
  --allowedTools "<read-only git rules>,Edit(//<abs run>/out/**)" \
  --add-dir <run>/out --settings <run>/reviewer-settings.json \
  --session-id <uuid> "<kickoff>"
```

- **Why `--restricted`.** Without it, the user's allow rules (for example `Bash(go test:*)`) would
  widen `Bash` beyond read-only git. `dontAsk` turns every request that no rule allows into a
  denial, so no permission prompt should appear.
- **Status hooks.** `--restricted` also drops the agterm status hooks. `reviewer-settings.json`
  carries hooks for `UserPromptSubmit` → `active`, `Stop` → `completed` and `Notification`
  (permission) → `blocked`. Each runs `'<agk>' review hook <state>`. That hidden subcommand sets
  the agent status of `$AGTERM_SESSION_ID` through `agterm.Ctl` (the companion's own id, with
  `--socket`). It is quoted like `companionCommand`.
- **Paths.** The settings file lives in the run root, which the reviewer can read but not write.

*Alternatives:*
- Pointing the hooks at agterm's `agterm-claude-status.sh`: that path belongs to agterm and is not
  ours to hard-code.
- Dropping `--restricted`: the user's allow rules leak in.

The `out` path is spliced into the permission-rule string, so a path holding rule syntax (`(`, `)`,
`,`), whitespace, control characters or glob metacharacters could add rules or widen the `Edit`
rule. `agk review start` refuses such a path for an interactive claude reviewer before the run
directory is created (`harness.CheckRulePath`). Inline claude and codex take no `Edit` rule and are
not affected.

Claude's cwd is the repository root, so its prompt asks for plain `git <subcommand>`, never
`git -C`. Those are exactly the command shapes the `Bash(git diff:*)`-style allow rules match. The
mandatory flags come after tuning here too.

**Spike first (task 1).** Before building this path, check interactively:
- `--restricted` works in the TUI;
- the `Edit(//abs/**)` allow rule lets `Write` create the output file and other writes are denied under `dontAsk`;
- the `--settings` hooks fire;
- the git commands the final prompt prescribes pass, including the read-rest command.

If any of these fails, stop and update the plan.

**Spike result (task 1.1, claude 2.1.319, TUI inside agterm).**
- **Works:**
  - `--restricted --strict-mcp-config --tools … --permission-mode dontAsk --allowedTools … --add-dir
    --settings` starts the TUI.
  - The `--settings` hooks fire: `UserPromptSubmit` and `Stop` each ran once per turn (log
    `active`, `completed`).
  - `git diff <mergebase>`, `git status` and `git diff <mergebase> -- .` (plain `git`) ran without a
    prompt.
  - A write to a repo file (`evil.txt`) and `go version` were denied without a prompt.
- **Fails: `Write(//<abs out>/**)` does not allow the write.** `Write(…)` allow rules are not matched
  by file permission checks. Claude prints `Permission allow rule (--allowed-tools): Write(…) is not
  matched by file permission checks — only Edit(path) rules are`, and `Write(out/1.json)` is
  denied under `dontAsk`. The rule must be `Edit(//<abs out>/**)` (Edit rules cover all
  file-editing tools). In `claude -p` with `Edit(//<abs out>/**)`, `Write` to `out/1.json` succeeds
  and `Write` to `evil.txt` in the repo root is denied.
- **Also seen:**
  - The TUI shows the workspace-trust prompt on first run in a directory, even with `--restricted`
    (the start handshake reports `started: false`; the existing scenario covers it).
  - `Read` of a file outside the cwd is denied in restricted mode. The kickoff must point at a file
    under the repository root, which `rounds/<k>/prompt.md` is.
  - The `Notification` hook (permission matcher) was not exercised: `dontAsk` never prompts.
- **Resolution:** the allow rule is `Edit(//<abs out>/**)`, and the tool list `--tools …,Write`
  stays. D4's argv and tasks 1.1 and 4.2 use it. The spec states only "writing under `out/`", so it
  is unchanged.

### D4a. Reviewer arguments cannot widen its permissions
`extra_args` of the reviewer role are refused when they touch the reviewer's boundary:
- **codex:** `-s`/`--sandbox`, `-a`/`--ask-for-approval`, `--full-auto`,
  `--dangerously-bypass-approvals-and-sandbox`, `--yolo`, `--add-dir`, `-C`/`--cd`, and `-c`/`--config`
  keys starting with `sandbox` or `approval`;
- **claude:** `--dangerously-skip-permissions`, `--permission-mode`, `--allowedTools`/`--allowed-tools`,
  `--tools`, `--add-dir`, `--settings`, `--setting-sources`, `--mcp-config`.

`agk review start` refuses them with an error naming the argument. This applies inline too: today
the inline reviewer passes them unchecked. Ordering (D3, D4) is the second line of defence, not the
only one.

### D5. Run state: `interactive` in `worker.json`; older runs are read-only
`reviewState` gains `Interactive bool`. `start` sets it in agterm.
- **`interactive` set:** `wait`, `send` and the prompts use the interactive path.
- **Inline (no companion):** unchanged.
- **Companion but no `interactive`:** started by an older agk. `wait` returns its `result.json`
  if present and otherwise exits 1, `send` refuses, `close` works.

Supporting the old mailbox protocol would keep `review run` and `nextRound` alive for a case that
only happens across a `brew upgrade` in the middle of a review.

### D6. Wait decision as a pure function; fix-ups counted on disk
`review.Decide(k, output []byte, exists bool, status string, waitErr error, fixups int)`
returns one of:
- `record completed`;
- `record blocked: invalid review output`;
- `type fix-up`;
- `needs-human` (`no-output` | `permission`);
- `cancelled`;
- `running`;
- `keep waiting`.

`ReviewWait` loops against one deadline fixed at its start (`--timeout`):
1. Read `result.json`; if present, print it.
2. Wait in slices (`waitSliced` over `out/<k>.json`) for `completed|blocked|idle`.
3. Decide, act, repeat.

**Each fix-up attempt gates like a round start.** Attempt n counts as started only once the agent
status `active` has been seen after its line was typed. The send handshake or a later `wait` sees
it and records `rounds/<k>/fixup-<n>.started`. Until then, `wait` applies these rules in order:
1. A valid `out/<k>.json` records the round as `completed`, as at any time.
2. A `blocked` status reports `needs-human`/`permission` at once: no new attempt and no
   `result.json`.
3. Everything else means keep waiting: `completed`/`idle` (it may be the previous turn's) and an
   invalid file, changed or not (a changed file may be half-written by the running attempt). It
   waits in slices for `active`, under the same deadline, and reports `running` when the deadline
   passes.

So an unconfirmed handshake never lets the previous turn's `completed` and an invalid file count as
the end of the fix-up turn. That would type a second fix-up into a running turn, or block the round
after fewer than three finished turns. A changed file never opens the gate.

Once the deadline has passed, the loop returns `running` after its last decision, even while an
invalid `out/<k>.json` exists. `waitPolling` checks the remaining budget before the file, so an
existing file can no longer turn a spent budget into an endless loop.

**Agent status belongs to the harness.** agk never sets the reviewer companion's agent status
(`Ctl.Status`) in interactive runs. It sets only the info line (`Ctl.Info`). Two consequences:
- `typeWhenIdle` and `wait` read the status as "turn running / over / permission prompt" and
  nothing else.
- A round recorded mid-turn leaves the status `active`, so `send` still waits for the turn to end.
  A round failed by validation leaves no `blocked` behind, so `send` is not mistaken for a
  permission prompt.

The inline path has no companion, so nothing changes there. `agk review hook` is the only agk
code that sets a reviewer's agent status, and it does so on behalf of claude's own hooks.

A fix-up writes `rounds/<k>/fixup-<n>.md`, which holds the fixed template plus the validator error,
increments `rounds/<k>/fixups` and types `Read <fixup-n.md> and follow it.` The limit is 2. The
count is on disk so that separate `wait` calls share it. A valid file seen mid-turn completes the
round at once, and the prompt tells the reviewer to write the file as its last step. The raw output
is copied to `rounds/<k>/output.json` on failure.

### D7. Shared worker plumbing
The companion helpers move from `apply.go` into `internal/cli/worker.go`: `handshake`,
`markStarted`/`roundStarted`, `waitPolling`/`waitSliced` (parameterised by the file that ends the
wait), and `typeWhenIdle`. `typeWhenIdle` waits for `completed|blocked|idle`, refuses on `blocked`,
types a line and runs the 30 s `active` handshake. `apply send`, `review send` and the fix-up use
`typeWhenIdle`. Behavior of `agk apply` does not change; its tests guard the move.

### D8. Prompts
The review, plan-review and re-review templates get an `Output` block:
- **interactive:** "write one JSON object matching `<schema.json>` to `<out/k.json>` as your last
  step; the repository root is `<root>`; read files by their path under the root";
- **inline:** the current "respond with a JSON object matching the provided output schema".

The git form is per harness:
- **codex:** `git -C <root>`; `ReadRestCommand(mergeBase, root)` renders `git -C <root> diff …`.
- **claude and inline:** plain `git` and the current read-rest command.

In interactive runs, round k>1 always uses the re-review wrapper, because the session is live.
`state.Session` matters only inline.

### D9. Removals
- The `review run` subcommand, `ReviewRun`, `nextRound`, the `pane` file and its checks in
  `send`.
- `MsgRead` and `MsgSend` leave `agterm.Ctl` and its fake if nothing else uses them.
- `Driver.Review`, `renderCodex` and the claude stream renderer stay for the inline path.

## Risks / Trade-offs

- [The schema is checked only after the fact] → `review.Parse` is the same validator as before,
  the fix-up loop corrects most slips, and after 2 fix-ups the round fails visibly.
- [The reviewer rewrites `out/k.json` after agk already recorded it] → the prompt asks for the
  write as the last step. A later rewrite is ignored, and the recorded `review.json` is
  authoritative.
- [The codex sandbox keys change in a future codex] → tests pin the argv. The spike result is in
  this design. Re-check with `codex sandbox` when bumping the minimum codex.
- [`dontAsk` or `--restricted` behaves differently in the claude TUI than in `-p`] → spike task 1
  gates the claude path.
- [An interactive codex reviewer loads the user's hooks, skills and memories (`bd prime` and
  others)] → it is noise, not a permission: the sandbox still bounds writes. The prompt already
  forbids loading review skills or running `agk`/`agtermctl`.
- [Each re-review prompt repeats the whole diff in a live context] → same as the current `resume`.
  The harness compacts. Not worse than today.
- [The user types into the reviewer while agk is about to type] → agk types only when the agent
  status says the turn is over, as for the applier.
- [The companion's agent status comes only from the harness, so a failed round does not turn the
  sidebar glyph `blocked`] → that is deliberate (D6). The info line names the failure, and
  `review wait` reports it.
- [A fix-up turn that starts and ends between two status polls, with `active` never seen, and
  leaves an invalid file] → `wait` keeps reporting `running`, because no `active` was seen and the
  file is not valid. The 30 s handshake right after typing makes a missed `active` rare. When it
  happens, the user sees the reviewer idle in its companion, and the main session reports the
  `running` timeout to them.
- [A future codex adds another flag or config key that widens the sandbox] → the D4a list is
  explicit and tested. The mandatory flags come after tuning, and `writable_roots=[]` is forced.

## Migration Plan

- Ship in the next agk release. `agk setup` needs no change, and the skills are relinked as usual.
- A review started by an older agk keeps working until its companion is closed: `wait` reads its
  `result.json` and `send` asks for a new review (D5).
- Rollback means reinstalling the previous agk. Run directories made by the new version are refused
  by the old `send` (no `pane` file), which is the same "start a new review" outcome.
