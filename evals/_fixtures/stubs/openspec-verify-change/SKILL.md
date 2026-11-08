---
name: openspec-verify-change
description: Verify that the implementation of an OpenSpec change matches its artifacts. Use when the user wants to validate a change before archiving. (Eval stub of the OpenSpec skill.)
---

# OpenSpec verify (eval stub)

This project's verification is scripted. For the change you are verifying (ask the user only if
none was named):

1. From the repository root run the checks:
   ```bash
   bash .eval/verify.sh <change>
   ```
   It prints the check results. Do not look for other evidence and do not change any file.
2. Write the verification report yourself, in your message: one line per check, then the issues
   grouped as CRITICAL, WARNING and SUGGESTION with their counts, then the final assessment
   (`ready for archive` when there is no CRITICAL issue, otherwise `N critical issue(s) to fix`).
   A failed check that blocks the change is CRITICAL; a passing check with a caveat is a WARNING.
