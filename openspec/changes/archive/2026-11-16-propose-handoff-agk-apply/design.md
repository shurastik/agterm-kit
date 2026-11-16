# Design

## Context

See proposal.md - Why. `agk-propose` runs the project's `openspec-propose` workflow in step 1, and
that workflow ends with its own handoff ("run `/opsx:apply` or ask me to apply this change"); the
eval stub of it does the same. `agk-explore` already overrides the nested workflow's handoff in its
"Handoff" section ("name `/agk-propose` instead; never name `/opsx:propose`"). The `agk-apply`
skill can be invoked by the model (its description has triggers, not "only when explicitly
invoked").

## Goals / Non-Goals

**Goals:**
- One handoff, `/agk-apply <change>`, whatever the nested workflow suggested.
- An eval that fails when either old handoff leaks into the last message.

**Non-Goals:**
- Changing `agk-apply` itself (its triggers or explicit-invocation policy).
- Changing `agk-explore`, `agk-archive`, the CLI or the OpenSpec skills shipped by `openspec update`.

## Decisions

- **Override in step 5, stated like `agk-explore`'s.** Step 5 says to name `/agk-apply <change>`
  instead of anything the `openspec-propose` workflow suggested (`/opsx:apply`, "ask me to apply",
  `agk apply start`), and never to name those. Step 1 already says not to stop at the nested
  workflow's suggestion; the override sits where the last message is written, so it wins.
  Alternative: only replace the text in step 5. Rejected: the nested workflow's own last line
  would still tempt the model to repeat `/opsx:apply`.
- **Forbid invoking `/agk-apply` explicitly, and test it with the skill available.** Naming a
  model-invocable skill next to "implement" invites the model to run it. The "never start"
  sentence names both `agk apply` and `/agk-apply`. In a real session `agk-apply` is installed, so
  the `plan-done` case plugin gets an `agk-apply` stub (`evals/_fixtures/stubs/agk-apply`) with
  the real skill's name and description, making it as discoverable and tempting as in production.
  Its body starts the applier through the stand-in (`agk apply start <change>`), so following it
  also trips the existing `no-apply` grader. A new `tool_used` grader (`tool: Skill`,
  `input_match: agk-apply`, `max: 0`) fails the case on the invocation itself, even when the
  agent then ends with the right message; the match also covers a namespaced `<plugin>:agk-apply`.
  Alternative: rely on `no-apply` alone. Rejected: an agent that invokes the skill and does not
  follow it would pass.
- **Negative graders as `match: not_contains` regexes on `last_message`**, as `idea-settled` does
  for `/opsx:propose`. Two separate graders (`/opsx:apply`, `agk apply start`) so a report names
  which old handoff leaked.

## Risks / Trade-offs

- [The model paraphrases, e.g. "run the agk-apply skill"] → the positive grader requires the
  literal `/agk-apply add-widget`, and the skill text gives that exact form.
- [Users outside Claude Code (Codex main session) have no `/agk-apply` slash command] → Codex
  invokes skills by name too (`$agk-apply`); the `agk-explore` handoff already names `/agk-propose`
  the same way, so the convention stays consistent.
