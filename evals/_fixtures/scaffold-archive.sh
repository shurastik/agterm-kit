#!/bin/bash
# Archive-case workspace, run by a case's scaffold.sh after scaffold-common.sh,
# in the workspace. Options, in any order:
#   --change         write the change add-widget and commit it
#   --code           leave an uncommitted code file, widget.go
#   --worktree       make the workspace a linked worktree of a main repository
#                    next to it (<workspace>.main, on main with the committed
#                    change), on branch add-widget with
#                    branch.add-widget.agkBase = main: what `agk apply start`
#                    leaves a session in. The stand-in agk logs and works in the
#                    workspace wherever it runs (git config eval.workspace)
#   --pending-worktree
#                    prepare worktree mode for the stand-in `agk apply start`
#                    when the case holds the `worktree` marker: a linked worktree
#                    on branch eval-worktree at .eval/worktree (hidden there, so
#                    the agent sees no worktree or branch of the change before the
#                    start; the agent's sandbox cannot create one itself), with
#                    .eval copied in, branch.add-widget.agkBase = main and
#                    eval.log pointing here
#   --main-branch    stand on branch add-widget with the base recorded in the
#                    workspace repository itself, which is a main checkout
#   --branch         the same without a recorded base
#   --run KIND:N:CHANGE
#                    pre-create .agk/KIND/N/ (KIND is applies or reviews) with a
#                    worker.json recording CHANGE, a companion left open
set -euo pipefail
fix="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
g() { git -c user.name=eval -c user.email=eval@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@"; }
while [ $# -gt 0 ]; do
  case "$1" in
    --change)
      bash "$fix/write-change.sh" >/dev/null
      g add -A
      g commit -qm "add-widget change"
      shift ;;
    --code)
      printf 'package widget\n\n// List prints the project widgets.\nfunc List() {}\n' > widget.go
      shift ;;
    --worktree)
      ws="$PWD"; main="$ws.main"; tmp="$ws.attach"
      rm -rf "$main" "$tmp"
      git clone -q "$ws" "$main"
      git -C "$main" remote remove origin
      git -C "$main" config user.name eval
      git -C "$main" config user.email eval@example.invalid
      git -C "$main" config commit.gpgsign false
      # attach the workspace: a worktree made next to it lends its .git file
      g -C "$main" worktree add -q -b add-widget "$tmp" main
      rm -rf "$ws/.git"
      mv "$tmp/.git" "$ws/.git"
      rm -rf "$tmp"
      g -C "$main" worktree repair "$ws" >/dev/null 2>&1
      git -C "$main" config branch.add-widget.agkBase main
      git -C "$main" config eval.workspace "$ws"
      shift ;;
    --pending-worktree)
      g worktree add -q -b eval-worktree .eval/worktree main
      mkdir -p .eval/worktree/.eval
      for f in .eval/*; do [ "$f" = .eval/worktree ] || cp -R "$f" .eval/worktree/.eval/; done
      git config branch.add-widget.agkBase main
      git config eval.log "$(pwd -P)"
      shift ;;
    --main-branch|--branch)
      g checkout -q -b add-widget
      # the commits the agent makes run with this repository's own config
      git config user.name eval
      git config user.email eval@example.invalid
      git config commit.gpgsign false
      if [ "$1" = --main-branch ]; then
        git config branch.add-widget.agkBase main
      fi
      shift ;;
    --run)
      [ $# -ge 2 ] || { echo "scaffold-archive: --run needs KIND:N:CHANGE" >&2; exit 1; }
      IFS=: read -r kind n change <<< "$2"
      case "$kind" in applies|reviews) ;; *) echo "scaffold-archive: bad run kind '$kind'" >&2; exit 1 ;; esac
      [[ "$n" =~ ^[0-9]+$ && -n "$change" ]] || { echo "scaffold-archive: bad --run '$2'" >&2; exit 1; }
      mkdir -p ".agk/$kind/$n"
      printf '{"worker":{"harness":"codex"},"change":"%s","interactive":true}\n' "$change" > ".agk/$kind/$n/worker.json"
      echo "COMP-$kind-$n" > ".agk/$kind/$n/companion"
      shift 2 ;;
    *) echo "scaffold-archive: unknown option '$1'" >&2; exit 1 ;;
  esac
done
