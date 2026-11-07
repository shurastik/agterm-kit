#!/bin/bash
# Runs the agk skill evals (evals/cases/) with `claude plugin eval`. Every case
# runs a real Claude child on your account and spends API usage; nothing here
# is part of `go test` or the release.
#
#   scripts/eval-skills.sh                         # whole suite, 3 runs per case
#   scripts/eval-skills.sh --case plan-done --runs 1
#
# The stand-in agk and revdiff go first on PATH: the agent inherits this PATH,
# and every case's scaffold refuses to run unless both resolve to them.
# AGK_EVAL_THRESHOLD (default: the baseline in evals/README.md) and
# AGK_EVAL_MAX_COST_USD override the score threshold and the cost ceiling;
# other arguments go to `claude plugin eval`.
set -euo pipefail
repo="$(cd "$(dirname "$0")/.." && pwd)"
threshold="${AGK_EVAL_THRESHOLD:-0.8}"
max_cost="${AGK_EVAL_MAX_COST_USD:-10}"
export PATH="$repo/evals/_fixtures/bin:$PATH"
cd "$repo"
bash evals/_fixtures/test.sh >/dev/null
exec claude plugin eval evals --scaffold --trust-plugin --no-publish --ablation none \
  --allow-tools Bash Write Edit --judge-model sonnet --threshold "$threshold" --max-cost-usd "$max_cost" "$@"
