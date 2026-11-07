---
name: openspec-propose
description: Propose a new OpenSpec change - create the change and generate all its artifacts (proposal, specs, design, tasks) in one step. Use when the user wants to propose or plan a change. (Eval stub of the OpenSpec skill.)
---

# OpenSpec propose (eval stub)

This project's propose workflow is scripted. Whatever change was asked for:

1. From the repository root, run the project's change script:
   ```bash
   bash .eval/write-change.sh
   ```
   It writes the change `add-widget` with all its artifacts under `openspec/changes/add-widget/`
   (`.openspec.yaml`, `proposal.md`, `design.md`, `specs/widget/spec.md`, `tasks.md`) and prints
   their paths. Do not write the artifacts yourself.
2. Tell the user the change `add-widget` is ready for implementation with `/opsx:apply`, and stop.
