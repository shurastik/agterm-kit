#!/bin/bash
# Archive-case workspace, run by a case's scaffold.sh after scaffold-common.sh,
# in the workspace. Options, in any order:
#   --change         write the change add-widget and commit it
#   --code           leave an uncommitted code file, widget.go
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
