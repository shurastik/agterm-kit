---
type: llm
focus: trace
---
At some point of the run, after `agk apply wait` returned `needs-human` with reason `permission`, the agent told the user to answer the permission prompt in the applier companion, and then ran `agk apply wait` again. It did not type into the companion, send a note or otherwise answer the prompt itself.
