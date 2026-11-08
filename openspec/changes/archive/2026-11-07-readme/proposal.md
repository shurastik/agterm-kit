# Proposal

## Why

agterm-kit is a public repository with a public Homebrew tap, yet it has no README: a visitor on
GitHub or `brew info` sees a file listing and a one-line description that still says "codex reviews
in companion sessions", from before the applier and the plan step existed. The only usage docs live
in AGENTS.md, which is written for coding agents, and the OpenSpec project context still describes
the retired HTML findings overlay. User-facing docs need one home, kept current by rule.

## What Changes

- Add `README.md` for people who have never seen agterm or agk: what agk is, how the main session,
  the applier and the reviewer work together (with a diagram), requirements, installation, a quick
  start (through the agent skills first, then the bare CLI), configuration (layers, keys, `agk
  config`, and the two known pitfalls: switching harness drops model/effort set by lower layers;
  `--applier`/`--reviewer` take `harness[:model]` and no effort), development, license.
- AGENTS.md: shrink the Install and Usage sections to a pointer at README.md, keep what agents need
  (commands, layout, conventions, workflow); add the missing `internal/termsafe/` row to Layout; add
  a convention that user-facing changes (commands, flags, config keys and their resolution,
  install/setup, skills) update README.md in the same change.
- `openspec/config.yaml`: rewrite `context` to the current system (applier and reviewer companions,
  harness drivers, TOML config, `termsafe` rendering; no HTML overlays, `html/template` or CSP);
  add an `operations.apply.guidance` entry to check README.md against the change's user-facing
  behavior before the review.
- Homebrew formula template (`packaging/homebrew/agk.rb.tmpl`): new `desc` matching the current
  scope. It reaches the tap with the next `scripts/release.sh`.
- GitHub repository description: the same text, set with `gh repo edit`.
- `cmd/agk/main.go`: fix the package doc comment, which still mentions overlays.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. No behavior changes: this is documentation and metadata only, so the change sets
`skip_specs: true` and is archived with `--skip-specs`.

## Impact

- Files: `README.md` (new), `AGENTS.md`, `openspec/config.yaml`, `packaging/homebrew/agk.rb.tmpl`,
  `cmd/agk/main.go` (comment only).
- External: the GitHub repository description of `shurastik/agterm-kit` (public, changed with
  `gh repo edit`); the tap formula's `desc` changes at the next release.
- No code paths, flags, config keys or specs change; `go test ./...` stays as it is.
