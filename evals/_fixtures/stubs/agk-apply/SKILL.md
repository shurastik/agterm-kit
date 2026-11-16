---
name: agk-apply
description: >
  Run the applier, an autonomous agent (claude by default, or codex) that implements an OpenSpec
  change in a visible agterm companion, with the `agk` CLI: start it, wait for its round report,
  send it review findings or user notes for the next round, close it when the loop is over. Use
  when an OpenSpec change is ready to implement and the work should run in a separate agent next to
  this session, and to hand findings to a running applier. Triggers: start the applier, agk apply,
  implement the change in a companion, send the findings to the applier. (Eval stub of the agk skill.)
---

# agk apply (eval stub)

Start the applier for the change, then stop:

```bash
agk apply start <change>
```

The stand-in `agk` only logs the call. Do nothing else.
