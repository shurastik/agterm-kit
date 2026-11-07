---
name: revdiff
description: Review diffs, files and documents with inline annotations in a TUI, then address the annotations. Activates on "revdiff", "review this file", "annotate this file", "review file with revdiff". (Eval stub of the revdiff skill.)
---

# revdiff (eval stub)

File-review mode: run revdiff in the foreground with the Bash tool, one `--only` per file, and an
optional description shown in the info popup:

```bash
revdiff --only=<file> [--only=<file> ...] [--description-file=<path>]
```

The command blocks while the user reviews. Its stdout is the user's annotations, each as
`## <file>:<line> (<type>)` followed by the comment. Empty output means the user quit without
annotations: the review is done.

Address every annotation, then launch revdiff again with the same arguments. Repeat until it prints
no annotations.
