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

# --- Diff reviews, the verify and archive scripts and the archive scaffold ---
# A second copy of the fixtures, with the outcomes the scaffold stages.
f2="$ws/fixtures2"
cp -R "$fix" "$f2"
stage() { # dir [scaffold-archive options...]: a workspace with the stand-ins first on PATH
  local d="$1"; shift
  mkdir -p "$d"
  (cd "$d" && PATH="$f2/bin:$PATH" bash "$f2/scaffold-common.sh" && PATH="$f2/bin:$PATH" bash "$f2/scaffold-archive.sh" "$@") || fail "scaffold-archive $*"
}
a() { PATH="$f2/bin:$PATH" EVAL_AGK_CASE="${CASE:-selftest-diff}" "$@"; }
# freshline DIR: "<fresh> <round>" of `agk review fresh`, after checking that the
# output carries a merge_base commit id.
freshline() {
  a agk review fresh "$1" | python3 -c 'import json,re,sys; r=json.load(sys.stdin); assert set(r)=={"fresh","round","merge_base"}, r; assert re.fullmatch("[0-9a-f]{40}", r["merge_base"]), r; print(str(r["fresh"]).lower(), r["round"])'
}
stage "$ws/c" --change --code --run applies:1:add-widget --run reviews:2:add-widget --run applies:3:other-change
cd "$ws/c"
for f in verify.sh archive.sh write-change.sh; do [ -x .eval/$f ] || fail "scaffold did not stage $f"; done
git log --format=%s | grep -qx 'add-widget change' || fail "scaffold-archive --change did not commit the change"
[ -f openspec/changes/add-widget/tasks.md ] || fail "scaffold-archive --change: no change files"
[ -f widget.go ] && ! git ls-files --error-unmatch widget.go >/dev/null 2>&1 || fail "scaffold-archive --code: widget.go missing or committed"
grep -q '"change":"add-widget"' .agk/applies/1/worker.json || fail "worker.json of applies/1"
grep -q '"change":"add-widget"' .agk/reviews/2/worker.json || fail "worker.json of reviews/2"
grep -q '"change":"other-change"' .agk/applies/3/worker.json || fail "worker.json of applies/3"
if (cd "$ws/c" && PATH="$f2/bin:$PATH" bash "$f2/scaffold-archive.sh" --bogus) 2>/dev/null; then fail "scaffold-archive accepted --bogus"; fi
if (cd "$ws/c" && PATH="$f2/bin:$PATH" bash "$f2/scaffold-archive.sh" --run nope:1:x) 2>/dev/null; then fail "scaffold-archive accepted a bad run kind"; fi

# review start --base: a run dir with a base, a fingerprint per round.
out="$(a agk review start --base main)" || fail "review start --base"
d1="$(printf '%s' "$out" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
[ "$(cat "$d1/base")" = main ] || fail "start --base: no base file in $d1"
[ -s "$d1/rounds/1/fingerprint" ] || fail "start --base: no fingerprint"
printf '%s' "$out" | grep -q '"started":true' || fail "start --base output: $out"
if a agk review start --base main --change add-widget 2>/dev/null; then fail "--base with --change accepted"; fi
if a agk review start --base no-such-ref 2>/dev/null; then fail "unknown base accepted"; fi
# --for records the change a diff review implements, in either spelling.
for fa in "--for add-widget" "--for=add-widget" "--base main --for add-widget"; do
  fo="$(a agk review start $fa)" || fail "review start $fa"
  fd="$(printf '%s' "$fo" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
  grep -q '"for":"add-widget"' "$fd/worker.json" || fail "start $fa: worker.json lacks for: $(cat "$fd/worker.json")"
  grep -q '"change"' "$fd/worker.json" && fail "start $fa: a diff review must record no change"
  [ "$(cat "$fd/base")" = main ] || fail "start $fa: base is not main"
done
if a agk review start --for ../secrets 2>/dev/null; then fail "--for ../secrets accepted"; fi
if a agk review start --for "" 2>/dev/null; then fail "empty --for accepted"; fi
if a agk review start --for= 2>/dev/null; then fail "empty --for= accepted"; fi
if a agk review start --for Add_X 2>/dev/null; then fail "non-kebab --for accepted"; fi
if a agk review start --change add-widget --for add-widget 2>/dev/null; then fail "--for with --change accepted"; fi

# wait plays the approve; fresh follows the workspace.
w="$(a agk review wait "$d1")"
printf '%s' "$w" | grep -q '"verdict":"approve"' || fail "diff wait: $w"
[ "$(freshline "$d1")" = "true 1" ] || fail "fresh should be true right after the round"
echo "package widget" > extra.go
[ "$(freshline "$d1")" = "false 1" ] || fail "fresh should turn false after a new file"
rm extra.go
[ "$(freshline "$d1")" = "true 1" ] || fail "fresh should be true again once the file is gone"

# send works on a diff run (no change dir), records a new fingerprint.
echo "note for round 2" > note2.md
s="$(a agk review send "$d1" --note note2.md)" || fail "send on a diff run"
printf '%s' "$s" | grep -q '"round":2' || fail "diff send: $s"
[ -s "$d1/rounds/2/fingerprint" ] || fail "send: no fingerprint for round 2"
[ "$(cat "$d1/rounds/2/merge-base")" = "$(cat "$d1/rounds/1/merge-base")" ] || fail "send: round 2 did not record the merge base"
if a agk review send "$d1" 2>/dev/null; then fail "send while round 2 still runs"; fi
[ "$(freshline "$d1")" = "true 2" ] || fail "fresh should follow the current round"

# fresh refuses a plan run and a bad dir; edit-during-review changes the code file.
po="$(a agk review start --change add-widget)"
pd="$(printf '%s' "$po" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
if a agk review fresh "$pd" 2>/dev/null; then fail "fresh accepted a plan run"; fi
if a agk review fresh "$ws" 2>/dev/null; then fail "fresh accepted a bad run dir"; fi
mkdir -p .eval/outcomes/selftest-edit
touch .eval/outcomes/selftest-edit/edit-during-review
CASE=selftest-edit
o3="$(a agk review start --base main)"
d3="$(printf '%s' "$o3" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
a agk review wait "$d3" >/dev/null
tail -1 widget.go | grep -q 'edited while the reviewer ran' || fail "edit-during-review did not touch widget.go"
[ "$(freshline "$d3")" = "false 1" ] || fail "fresh should be false after edit-during-review"
CASE=selftest-diff

# close logs the run dir, for diff, plan and pre-created runs.
a agk review close "$d1" || fail "close diff run"
a agk review close .agk/reviews/2 || fail "close pre-created review run"
a agk apply close .agk/applies/1 || fail "apply close"
if a agk apply close "$ws" 2>/dev/null; then fail "apply close accepted a non-run dir"; fi

# nothing to review: no diff and no untracked file (and a clean tree with a code file is reviewable).
stage "$ws/e" --change
if (cd "$ws/e" && a agk review start --base main) 2>"$ws/e.err"; then fail "start --base on a clean tree"; fi
grep -q 'nothing to review' "$ws/e.err" || fail "nothing to review not reported: $(cat "$ws/e.err")"
[ ! -d "$ws/e/.agk/reviews" ] || [ -z "$(ls "$ws/e/.agk/reviews")" ] || fail "nothing to review left a run dir"
grep -q '^agk review start --base main$' "$ws/e/agk-calls.log" || fail "the refused start was not logged"

# verify.sh and archive.sh.
v="$(cd "$ws/c" && a bash .eval/verify.sh add-widget)"
printf '%s' "$v" | grep -q 'pass' && ! printf '%s' "$v" | grep -qi 'fail' || fail "default verify results: $v"
mkdir -p .eval/outcomes/selftest-diff
printf 'correctness: requirement "List widgets" has no implementation: FAIL\n' > .eval/outcomes/selftest-diff/verify.txt
v="$(a bash .eval/verify.sh add-widget)"
printf '%s' "$v" | grep -q 'FAIL' || fail "verify.txt not printed: $v"
rm .eval/outcomes/selftest-diff/verify.txt
mkdir -p .eval/outcomes/selftest-verify-edit && touch .eval/outcomes/selftest-verify-edit/edit-during-verify
o4="$(a agk review start --base main)"
d4="$(printf '%s' "$o4" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
[ "$(freshline "$d4")" = "true 1" ] || fail "fresh should be true before verify edits"
CASE=selftest-verify-edit a bash .eval/verify.sh add-widget >/dev/null
tail -1 widget.go | grep -q 'edited while verify ran' || fail "edit-during-verify did not touch widget.go"
[ "$(freshline "$d4")" = "false 1" ] || fail "fresh should be false after edit-during-verify"
if a bash .eval/verify.sh 2>/dev/null; then fail "verify.sh without a change"; fi
touch .eval/outcomes/selftest-diff/archive-fail
if a bash .eval/archive.sh add-widget 2>/dev/null; then fail "archive.sh should fail with archive-fail"; fi
[ -d openspec/changes/add-widget ] || fail "failed archive moved the change"
rm .eval/outcomes/selftest-diff/archive-fail
a bash .eval/archive.sh add-widget >/dev/null || fail "archive.sh"
[ ! -d openspec/changes/add-widget ] && [ -f openspec/changes/archive/2026-11-08-add-widget/proposal.md ] || fail "archive.sh did not move the change"
cl="$(cat agk-calls.log)"
for pat in '^verify add-widget$' '^archive add-widget$' '^agk review fresh .*reviews/1$' '^agk review close .*reviews/1$' '^agk apply close \.agk/applies/1$' '^agk review start --base main$'; do
  printf '%s\n' "$cl" | grep -qE "$pat" || fail "agk-calls.log lacks /$pat/:
$cl"
done
# A diff round records its merge base, and fresh compares against it: committing
# the reviewed work on main keeps the round fresh, an edit or unrelated history does not.
cd "$ws"
stage "$ws/mb" --change --code
cd "$ws/mb"
omb="$(a agk review start --base main)"
dmb="$(printf '%s' "$omb" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
m0="$(git rev-parse HEAD)"
[ "$(cat "$dmb/rounds/1/merge-base")" = "$m0" ] || fail "start --base: rounds/1/merge-base is not the merge base $m0"
[ "$(a agk review fresh "$dmb" | python3 -c 'import json,sys; print(json.load(sys.stdin)["merge_base"])')" = "$m0" ] || fail "fresh does not print the recorded merge base"
git -c user.name=t -c user.email=t@t -c commit.gpgsign=false -c core.hooksPath=/dev/null add widget.go
git -c user.name=t -c user.email=t@t -c commit.gpgsign=false -c core.hooksPath=/dev/null commit -qm "widget on main"
[ "$(freshline "$dmb")" = "true 1" ] || fail "fresh should stay true after committing the reviewed file on main"
if a agk review start --base main 2>/dev/null; then fail "start --base main after the commit should find nothing to review"; fi
echo "// edited" >> widget.go
[ "$(freshline "$dmb")" = "false 1" ] || fail "fresh should be false after an edit"
git checkout -q -- widget.go
[ "$(freshline "$dmb")" = "true 1" ] || fail "fresh should be true again after the edit is reverted"
git -c user.name=t -c user.email=t@t -c commit.gpgsign=false -c core.hooksPath=/dev/null checkout -q --orphan unrelated
git -c user.name=t -c user.email=t@t -c commit.gpgsign=false -c core.hooksPath=/dev/null commit -qm "unrelated" >/dev/null
[ "$(freshline "$dmb")" = "false 1" ] || fail "fresh should be false once the recorded merge base is no longer an ancestor of HEAD"
# a missing recorded merge base is an error; an old round without the file falls back to the base
echo "0000000000000000000000000000000000000000" > "$dmb/rounds/1/merge-base"
if a agk review fresh "$dmb" 2>"$ws/mb.err"; then fail "fresh accepted a missing merge base"; fi
grep -q 'is not in the repository' "$ws/mb.err" || fail "missing merge base message: $(cat "$ws/mb.err")"
rm "$dmb/rounds/1/merge-base"
a agk review fresh "$dmb" | grep -q '"merge_base":"' || fail "fresh without a merge-base file must still print merge_base"
# the retry-nothing-to-review marker: a retry at a commit id finds nothing too
git checkout -q main
mkdir -p .eval/outcomes/selftest-retry && touch .eval/outcomes/selftest-retry/retry-nothing-to-review
echo "// again" >> widget.go
CASE=selftest-retry a agk review start --base main >/dev/null || fail "start --base main with the retry marker"
if CASE=selftest-retry a agk review start --base "$(git rev-parse HEAD)" 2>/dev/null; then fail "retry-nothing-to-review: a start at a commit id should find nothing to review"; fi
git checkout -q -- widget.go
cd "$ws"

# --- Worktree stand-ins: scaffold --worktree/--branch, agk sync, agk finish ---
cd "$ws"
# the git stand-in runs the real binary, and never itself
gv="$(PATH="$f2/bin:$PATH" git --version)" || fail "the git stand-in failed"
case "$gv" in "git version "*) ;; *) fail "the git stand-in printed: $gv" ;; esac
[ "$(export PATH="$f2/bin:$PATH"; hash -r; command -v git)" = "$f2/bin/git" ] || fail "the git stand-in is not first on PATH"
stage "$ws/w" --change --worktree --code
cd "$ws/w"
[ "$(git rev-parse --abbrev-ref HEAD)" = add-widget ] || fail "scaffold --worktree is not on branch add-widget"
[ "$(git config --get branch.add-widget.agkBase)" = main ] || fail "scaffold --worktree: no agkBase"
git log main --format=%s | grep -qx 'add-widget change' || fail "scaffold --worktree: main lacks the plan commit"
[ "$(git rev-parse main)" = "$(git rev-parse HEAD)" ] || fail "scaffold --worktree: the branch should start at main"
[ -f widget.go ] && ! git ls-files --error-unmatch widget.go >/dev/null 2>&1 || fail "scaffold --worktree --code: widget.go missing or committed"
[ "$(git config --get commit.gpgsign)" = false ] && [ -n "$(git config --get user.email)" ] || fail "scaffold --worktree: no commit identity"
# a real linked worktree of a main repository next to the workspace
[ "$(git rev-parse --path-format=absolute --git-dir)" != "$(git rev-parse --path-format=absolute --git-common-dir)" ] || fail "scaffold --worktree: the workspace is not a linked worktree"
[ "$(git -C "$ws/w.main" rev-parse --abbrev-ref HEAD)" = main ] || fail "scaffold --worktree: the main checkout is not on main"
git -C "$ws/w.main" worktree list --porcelain | grep -q "^worktree $(pwd -P)$" || fail "scaffold --worktree: the workspace is not in the main repository's worktree list"
[ -z "$(git status --porcelain)" ] || [ "$(git status --porcelain)" = "?? widget.go" ] || fail "scaffold --worktree: unexpected status: $(git status --porcelain)"
# sync refuses uncommitted files, and finish refuses an unarchived change
if a agk sync add-widget 2>"$ws/w.err"; then fail "sync accepted uncommitted files"; fi
grep -q 'widget.go' "$ws/w.err" || fail "sync did not name the uncommitted file: $(cat "$ws/w.err")"
if a agk finish add-widget 2>/dev/null; then fail "finish accepted uncommitted files"; fi
git add widget.go && git commit -qm "feat: widget list"
if a agk finish add-widget 2>"$ws/w.err"; then fail "finish accepted an unarchived change"; fi
grep -q 'not archived' "$ws/w.err" || fail "finish: $(cat "$ws/w.err")"
# sync: rebased false by default, true on the marked call, once
s1="$(a agk sync add-widget)" || fail "sync"
printf '%s' "$s1" | grep -q '"rebased":false' && printf '%s' "$s1" | grep -q '"base":"main"' && printf '%s' "$s1" | grep -q '"change":"add-widget"' || fail "sync output: $s1"
printf '%s' "$s1" | python3 -c 'import json,sys; r=json.load(sys.stdin); assert set(r)=={"change","base","pulled","rebased","head"}, r' || fail "sync keys: $s1"
a agk sync add-widget --no-pull >/dev/null || fail "sync --no-pull"
if a agk sync 2>/dev/null; then fail "sync without a change"; fi
if a agk sync Bad_Name 2>/dev/null; then fail "sync accepted a bad name"; fi
grep -qx '  rebased: false' agk-calls.log || fail "sync did not log its result"
mkdir -p .eval/outcomes/selftest-rebase && touch .eval/outcomes/selftest-rebase/sync-rebased-2
: > agk-calls.log # the stand-in numbers the sync calls by their lines in the log
o5="$(a agk review start --base main)"
d5="$(printf '%s' "$o5" | python3 -c 'import json,sys; print(json.load(sys.stdin)["dir"])')"
[ "$(freshline "$d5")" = "true 1" ] || fail "fresh before a rebasing sync"
rb() { CASE=selftest-rebase a agk sync add-widget; }
printf '%s' "$(rb)" | grep -q '"rebased":false' || fail "sync-rebased-2: the first call must not rebase"
[ "$(freshline "$d5")" = "true 1" ] || fail "fresh after a non-rebasing sync"
printf '%s' "$(rb)" | grep -q '"rebased":true' || fail "sync-rebased-2: the second call must rebase"
[ "$(freshline "$d5")" = "false 1" ] || fail "fresh should turn false after a rebasing sync"
printf '%s' "$(rb)" | grep -q '"rebased":false' || fail "sync-rebased-2: only the second call rebases"
mkdir -p .eval/outcomes/selftest-conflict && touch .eval/outcomes/selftest-conflict/sync-conflict
if CASE=selftest-conflict a agk sync add-widget 2>"$ws/w.err"; then fail "sync-conflict should exit 1"; fi
grep -q 'conflicts in widget.go' "$ws/w.err" || fail "sync-conflict message: $(cat "$ws/w.err")"
# archive, commit, finish; finish-fail refuses
a bash .eval/archive.sh add-widget >/dev/null || fail "archive.sh in the worktree workspace"
if a agk finish add-widget 2>"$ws/w.err"; then fail "finish accepted an uncommitted archive"; fi
git add -A && git commit -qm "chore(openspec): archive add-widget"
mkdir -p .eval/outcomes/selftest-finish-fail && touch .eval/outcomes/selftest-finish-fail/finish-fail
if CASE=selftest-finish-fail a agk finish add-widget 2>"$ws/w.err"; then fail "finish-fail should exit 1"; fi
grep -q 'agk sync add-widget' "$ws/w.err" || fail "finish-fail message: $(cat "$ws/w.err")"
f1="$(cd "$ws/w.main" && a agk finish add-widget)" || fail "finish from the main checkout"
printf '%s' "$f1" | python3 -c 'import json,sys; r=json.load(sys.stdin); assert set(r)=={"change","base","head","closed"} and r["base"]=="main", r' || fail "finish output: $f1"
grep -q '^agk finish add-widget$' agk-calls.log || fail "finish was not logged"
# a file changing right after the n-th sync (edit-after-sync-<n>)
mkdir -p .eval/outcomes/selftest-edit-sync && touch .eval/outcomes/selftest-edit-sync/edit-after-sync-1
git status --porcelain | grep -q . && fail "the workspace is not clean before the edit-after-sync test"
: > agk-calls.log
CASE=selftest-edit-sync a agk sync add-widget >/dev/null || fail "sync with edit-after-sync-1"
tail -1 widget.go | grep -q 'edited after the sync' || fail "edit-after-sync-1 did not touch widget.go"
git checkout -q -- widget.go
# a change branch checked out in a main checkout: no linked worktree
stage "$ws/m" --change --main-branch --code
[ "$(git -C "$ws/m" rev-parse --path-format=absolute --git-dir)" = "$(git -C "$ws/m" rev-parse --path-format=absolute --git-common-dir)" ] || fail "scaffold --main-branch made a linked worktree"
[ "$(git -C "$ws/m" config --get branch.add-widget.agkBase)" = main ] || fail "scaffold --main-branch: no agkBase"
[ "$(git -C "$ws/m" rev-parse --abbrev-ref HEAD)" = add-widget ] || fail "scaffold --main-branch is not on add-widget"
# a branch without a recorded base: sync and finish refuse
stage "$ws/x" --change --branch --code
if (cd "$ws/x" && a agk sync add-widget) 2>"$ws/x.err"; then fail "sync accepted a branch without agkBase"; fi
grep -q 'agkBase' "$ws/x.err" || fail "sync without agkBase: $(cat "$ws/x.err")"
if (cd "$ws/x" && a agk finish add-widget) 2>/dev/null; then fail "finish accepted a branch without agkBase"; fi
[ "$(git -C "$ws/x" config --get commit.gpgsign)" = false ] || fail "scaffold --branch: no commit identity"
[ -z "$(git -C "$ws/x" config --get branch.add-widget.agkBase)" ] || fail "scaffold --branch recorded a base"
[ "$(git -C "$ws/x" rev-parse --abbrev-ref HEAD)" = add-widget ] || fail "scaffold --branch is not on add-widget"
cd "$ws"

cd "$ws/b"
[ "$(manifest)" = "$before" ] || fail "the checkout's outcomes/ changed during the self-test"
echo "ok"
