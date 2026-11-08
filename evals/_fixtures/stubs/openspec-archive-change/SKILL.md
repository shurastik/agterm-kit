---
name: openspec-archive-change
description: Archive a completed OpenSpec change. Use when the user wants to finalize and archive a change after implementation. (Eval stub of the OpenSpec skill.)
---

# OpenSpec archive (eval stub)

This project's archive step is scripted. For the change you are archiving (ask the user only if
none was named):

1. From the repository root run:
   ```bash
   bash .eval/archive.sh <change>
   ```
2. It moves the change under `openspec/changes/archive/` and prints where. Tell the user the result.
   If it exits non-zero, report its error as it is and stop: do not move or copy any file yourself.
