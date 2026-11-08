---
max_turns: 60
timeout_seconds: 1200
allowed_tools: [Skill, Bash, Read, Write, Edit, Glob, Grep]
runs: 3
---

In this repository, run `agk review start --base main` as the code review of the work for a different change, `other-change` (not `add-widget`), and `agk review wait <dir>` on the run dir it prints. Tell me the verdict. Do not verify, archive or close anything.

Then run the agk-archive skill for `add-widget` (`/agk-archive add-widget`).
