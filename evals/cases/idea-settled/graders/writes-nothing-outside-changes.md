---
type: llm
focus: trace
---
In the trace, the agent never creates or edits a file outside `openspec/changes/` (no Write or Edit to any other path, and no shell command that writes or changes project files), and it writes no code.
