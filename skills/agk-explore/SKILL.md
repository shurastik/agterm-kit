---
name: agk-explore
description: >
  Think an idea through with the project's OpenSpec explore workflow, then hand it to /agk-propose
  instead of the stock `openspec-propose` handoff, so the plan goes through the reviewer. It
  follows the project's `openspec-explore` skill unchanged and only swaps the handoff. Use only
  when explicitly invoked: /agk-explore, "agk explore".
---

# agk explore

The brainstorm step of the workflow: the project's OpenSpec explore workflow, with one override at
the end. Every step runs in this (main) session; no `agk` command is needed.

## 0. Precondition

The project must have the OpenSpec `openspec-explore` skill: a skill named `openspec-explore` in
your skill list, also under a plugin namespace (`<plugin>:openspec-explore`), or
`.claude/skills/openspec-explore/SKILL.md` in the repo (Codex: the skill of that name). If it is
missing, stop and tell the user: run `openspec init` in a project without `openspec/`, or
`openspec update` in one that has it, then invoke `/agk-explore` again. Run no command and create
no change.

## 1. Explore

Follow the project's `openspec-explore` skill for the whole exploration, passing on what the user
gave you with the invocation. Keep all of its guardrails: you are a thinking partner, you write no
code, and you capture anything into change artifacts only when the user confirms. Do not copy or
paraphrase that skill here; it is kept current by `openspec update`.

## 2. Handoff

Whenever the exploration is ready to become a change, or a change was captured, name
**`/agk-propose`** as the only next step, whatever the project's `openspec-explore` skill suggests.
Never name any other way to start the proposal or the implementation, whether a slash command, a
skill name or an "ask me to" phrase, for example the `openspec-propose` or `openspec-apply-change`
skill, in the last message or in the closing summary. `/agk-propose` writes the change and has the
reviewer check the plan before anyone implements it.

Only name the next step. Never start `/agk-propose`, `agk review` or `agk apply` yourself.
