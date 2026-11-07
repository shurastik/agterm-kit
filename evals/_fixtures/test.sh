#!/bin/bash
# Self-test of the eval fixtures: drives the stand-in agk and revdiff through
# one review loop and checks the scaffold PATH guard. No model, no cost.
set -euo pipefail
fix="$(cd "$(dirname "$0")" && pwd)"
fail() { echo "FAIL: $*" >&2; exit 1; }
ws="$(mktemp -d)"
trap 'rm -rf "$ws"' EXIT
manifest() { (cd "$fix" && find outcomes -type f -print0 | sort -z | xargs -0 shasum); }
before="$(manifest)"
# guard N CMD...: run CMD killed after N seconds, so a hang fails the test (exit 142).
guard() { local n="$1"; shift; perl -e 'alarm shift; exec @ARGV or exit 127' "$n" "$@"; }

# PATH guard: a foreign agk first on PATH fails the scaffold, the stand-ins pass.
mkdir -p "$ws/foreign" "$ws/a"
printf '#!/bin/sh\necho real agk\n' > "$ws/foreign/agk"; chmod +x "$ws/foreign/agk"
if (cd "$ws/a" && PATH="$ws/foreign:$fix/bin:$PATH" bash "$fix/scaffold-common.sh") 2>/dev/null; then
  fail "scaffold passed with a foreign agk first on PATH"
fi
# The real agk, when installed, is refused the same way.
if real="$(PATH="${PATH//$fix\/bin:/}" command -v agk)"; then
  mkdir -p "$ws/r"
  if (cd "$ws/r" && PATH="$(dirname "$real"):$fix/bin:$PATH" bash "$fix/scaffold-common.sh") 2>/dev/null; then
    fail "scaffold passed with the real agk ($real) first on PATH"
  fi
fi
# The loop runs on a copy of the fixtures, so the checkout is never touched.
fx="$ws/fixtures"
cp -R "$fix" "$fx"
mkdir -p "$ws/b"
(cd "$ws/b" && PATH="$fx/bin:$PATH" bash "$fx/scaffold-common.sh") || fail "scaffold failed with the stand-ins first on PATH"
[ -d "$ws/b/.git" ] || fail "scaffold did not make a git repo"
# The agent's sandbox cannot read the repo's case and fixture dirs, so the
# scaffold copies what the stand-ins and stubs need into the workspace.
[ -x "$ws/b/.eval/write-change.sh" ] || fail "scaffold did not stage write-change.sh"
[ -f "$ws/b/.eval/outcomes/selftest/round-1.json" ] || fail "scaffold did not stage the outcomes"

# One review loop: findings, send with a note, approve, revdiff, close.
cd "$ws/b"
export PATH="$fx/bin:$PATH" EVAL_AGK_CASE=selftest
bash .eval/write-change.sh >/dev/null
# The stand-ins read the staged copy: drop the fixture copy's outcomes.
rm -rf "$fx/outcomes"

out="$(agk review start --change add-widget)"
dir="$(printf '%s' "$out" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
[ -d "$dir" ] || fail "start: no run dir in $out"
printf '%s' "$out" | grep -q '"companion"' || fail "start: no companion in $out"

w1="$(agk review wait "$dir")"
printf '%s' "$w1" | grep -q '"status":"completed"' || fail "wait 1: $w1"
printf '%s' "$w1" | grep -q '"round":1' || fail "wait 1 round: $w1"
printf '%s' "$w1" | grep -q '"findings":1' || fail "wait 1 findings: $w1"
python3 - "$dir/rounds/1/review.json" <<'PY' || fail "round 1 review.json invalid"
import json, sys
r = json.load(open(sys.argv[1]))
assert r["verdict"] == "needs-attention" and len(r["findings"]) == 1, r
for k in ("severity", "title", "body", "file", "line_start", "line_end", "confidence", "recommendation"):
    assert k in r["findings"][0], k
PY

if agk review send "$dir" --note /nonexistent 2>/dev/null; then fail "send accepted a missing note"; fi
echo "Fixed the task in tasks.md." > note.md
echo "- [ ] 1.1 fixed" >> openspec/changes/add-widget/tasks.md
s="$(agk review send "$dir" --note note.md)"
printf '%s' "$s" | grep -q '"round":2' || fail "send: $s"
if agk review send "$dir" 2>/dev/null; then fail "send while round 2 still runs"; fi

w2="$(agk review wait "$dir")"
printf '%s' "$w2" | grep -q '"verdict":"approve"' || fail "wait 2: $w2"
printf '%s' "$w2" | grep -q '"findings":0' || fail "wait 2 findings: $w2"
python3 -c 'import json,sys; r=json.load(open(sys.argv[1])); assert r["verdict"]=="approve" and r["findings"]==[]' "$dir/rounds/2/review.json" || fail "round 2 review.json"

a1="$(revdiff --only=openspec/changes/add-widget/proposal.md --description-file="$dir/revdiff-description.md")"
printf '%s' "$a1" | grep -q 'design.md' || fail "revdiff 1 annotations: $a1"
a2="$(revdiff --only=openspec/changes/add-widget/proposal.md)"
[ -z "$a2" ] || fail "revdiff 2 should print no annotations: $a2"

# A flag without its value fails fast instead of looping.
for args in "review start --change" "review start --reviewer" "review send $dir --note"; do
  set +e; guard 5 agk $args >/dev/null 2>&1; rc=$?; set -e
  [ "$rc" -ne 0 ] && [ "$rc" -ne 142 ] || fail "agk $args: exit $rc, want a prompt non-zero exit"
done

agk review close "$dir" || fail "close"
agk apply start add-widget >/dev/null 2>&1 || true

log="$(cat agk-calls.log)"
expect() { printf '%s\n' "$log" | grep -qE "$1" || fail "log lacks /$1/:
$log"; }
expect '^agk review start --change add-widget$'
expect '^  files: \.openspec\.yaml design\.md proposal\.md specs/widget/spec\.md tasks\.md'
expect '^agk review wait .*\.agk/reviews/1$'
expect '^agk review send .*--note note\.md$'
expect '^  note: Fixed the task in tasks\.md\.$'
expect '^  changed since round 1: .*tasks\.md'
expect '^revdiff --only=openspec/changes/add-widget/proposal\.md --description-file=.*$'
expect '^  revdiff: description file is the run dir'"'"'s revdiff-description\.md$'
expect '^agk review close .*\.agk/reviews/1$'
expect '^  close: last verdict approve$'
expect '^agk apply start add-widget$'
[ "$(manifest)" = "$before" ] || fail "the checkout's outcomes/ changed during the self-test"
echo "ok"
