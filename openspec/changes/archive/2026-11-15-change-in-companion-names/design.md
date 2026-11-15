# Design

## Context

Both workers build their companion name inline, right before `startCompanion`:
`fmt.Sprintf("%s-apply #%d", w.Harness, num)` in `internal/cli/apply.go` and
`fmt.Sprintf("%s-review #%d", w.Harness, num)` in `internal/cli/review.go`. The name reaches
`agtermctl session new --name` as its own argv element (`agterm.CLI.NewCompanion` execs agtermctl
directly), not through the login shell that runs `--command`, so it is not quoted and needs no
escaping.

What each start knows about the change:

```
agk apply start <change>          change: positional, kebab-case checked
agk review start --change <c>     change: reviewState.Change, kebab-case checked -> plan review
agk review start --base <ref>     nothing                                       -> diff review
```

`reviewState.Change != ""` is what makes a run a plan review (the prompt renderer, `send` and
`fresh` all branch on it), so a diff review cannot reuse that field for its change.

## Goals / Non-Goals

**Goals:**
- Every companion of a change carries the change name. A diff review carries it when the caller
  names the change.
- A plan review and a code review of the same change can be told apart by name.

**Non-Goals:**
- Renaming the agent's own session. agterm shows the live terminal title of a session that has no
  explicit name, and an explicit name would hide it. agk would also have to restore the name after
  the change, and `agtermctl session rename` has no documented way back to the title.
- Using the recorded `for` field anywhere else, for example in `/agk-archive`'s search for the
  change's latest review run. The field makes that possible later.
- Putting the change into info lines (`codex #8 r1: …`) or the stderr review log.

## Decisions

**Name format: `<change> · <harness>-<kind> #N`.** The change goes first because the sidebar
truncates long names at the end, and the change is what tells companions apart. The rest of the
name is the old one, so `#N` still matches the info lines. The separator is ` · ` (U+00B7 between
spaces). It reaches agtermctl as argv, so the fish restriction on `--command` values does not apply.
Considered: the change as a suffix (`codex-review #8 add-auth`), which gets cut off first in a
narrow sidebar.

**One helper builds every name.** A small function `companionName(change, harness, kind, num)`
returns `<harness>-<kind> #N` for an empty change and the prefixed form otherwise. Both start paths
call it, so the format lives in one place and has one table test.

**Plan reviews are named `-plan`.** A plan review and a code review share the review numbering, so
`codex-review #8` and `codex-review #9` could be either. The plan kind costs nothing. Info lines and
the review log stay `codex #8 …`, as they are today.

**`--for <change>` instead of autodetection.** The agent that starts a code review knows which
change it implements, so an explicit flag never guesses wrong. Autodetecting the single active
change from `openspec list` would add an `openspec` call to `review start` and silently give no name
when zero or two changes are active. `--change` cannot carry it: that flag selects a plan review.

**Recorded as `"for"` in `worker.json` (`reviewState.For`).** It is a separate field, so every
existing `Change != ""` branch stays a plan-review test. It has `omitempty`, so run dirs written by
an older agk or started without `--for` read as before, with no migration.

**Validation: kebab-case only, no existence check.** `--for` labels a run and changes no file that
is read. The same `changeName` regexp as `--change` keeps path-like or empty values out. A typo only
yields a wrong label in the sidebar. Requiring `openspec/changes/<change>/` would refuse a review
right after an archive moved the directory. `--for` together with `--change` is refused before any
run directory exists, like `--change` with `--base`. An empty `--for ""` counts as given, by the same
`fs.Visit` rule `--change` uses, and fails the regexp.

**Skills pass `--for`.** `agk-review` adds `--for <change>` when the diff is the implementation of
an OpenSpec change it knows, and leaves it out otherwise. `agk-archive` always knows its change, so
its fallback review passes it. The stand-in agk of the evals parses `--for` and records it, and the
`fresh-session-archive` grader checks for it.

## Risks / Trade-offs

- [`implementation-worktree`, archived first, also modified the apply-worker requirement "Applier
  companion inside agterm"] → this change's delta carries the archived text (worktree root,
  `worktree`/`pulled` JSON, the worktree scenario) with only the name changed.
- [Long change names push `#N` out of a narrow sidebar] → accepted; the change name is the more
  useful half, and the info line inside the companion still shows `#N`.
- [Older agk skills call `review start` without `--for`] → the flag is optional, and those reviews
  keep today's name.
