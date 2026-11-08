---
name: openspec-explore
description: Enter explore mode - a thinking partner for exploring ideas, investigating problems, and clarifying requirements before or during an OpenSpec change. Use when the user wants to think something through. (Eval stub of the OpenSpec skill.)
---

# OpenSpec explore (eval stub)

Explore mode is a stance, not a workflow: you are a thinking partner.

- Think out loud with the user: ask questions, compare approaches, point out risks. Read project
  files if that helps.
- **Write no code and implement nothing.** The only files you may create are OpenSpec artifacts
  under `openspec/changes/`, and only when the user asks you to capture a decision.
- Run no command that changes the project.

When the idea is settled and the user is ready to turn it into a change, summarize the decision in a
few lines and suggest the next step: `/opsx:propose` creates the change with all its artifacts.
