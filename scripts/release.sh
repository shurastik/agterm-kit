#!/usr/bin/env bash
# Release agk: tag vVERSION, then render the Homebrew formula into the tap and push it.
#
#   scripts/release.sh 0.1.0
#   DRY_RUN=1 scripts/release.sh 0.1.0   # checks + rendered formula, no tag, no push
#
# Safe to rerun after a partial failure: an existing vVERSION tag on HEAD is reused,
# and an already-published formula is left as is.
# AGK_TAP_DIR is the shurastik/homebrew-tap checkout (default ~/Projects/homebrew-tap).
set -euo pipefail

repo="shurastik/agterm-kit"
tap_dir="${AGK_TAP_DIR:-$HOME/Projects/homebrew-tap}"
tap_branch="main" # the branch `brew update` reads
dry_run="${DRY_RUN:-}"

die() { echo "release: $*" >&2; exit 1; }

[[ $# -eq 1 ]] || die "usage: scripts/release.sh X.Y.Z"
version="$1"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be X.Y.Z, got '$version'"
tag="v$version"

root="$(git rev-parse --show-toplevel)"
cd "$root"
template="packaging/homebrew/agk.rb.tmpl"

# Everything that can fail locally is checked before the tag is published.
check_tap() {
  [[ -d "$tap_dir/.git" ]] || die "tap checkout not found at $tap_dir (set AGK_TAP_DIR)"
  [[ "$(git -C "$tap_dir" branch --show-current)" == "$tap_branch" ]] || die "tap checkout is not on $tap_branch"
  git -C "$tap_dir" remote get-url origin >/dev/null 2>&1 || die "tap checkout has no origin remote"
  local dirty
  dirty="$(git -C "$tap_dir" status --porcelain --untracked-files=all)"
  # A rerun after a failed tap commit finds its own formula left behind. Drop it only when both the
  # working copy and the index hold exactly what this script renders for this version (or HEAD's
  # copy); anything else is a hand edit and stops the release.
  if [[ -n "$dirty" && -n "$tag_exists" && "$dirty" == "$(git -C "$tap_dir" status --porcelain --untracked-files=all -- Formula/agk.rb)" ]] &&
    own_formula <(cat "$tap_dir/Formula/agk.rb" 2>/dev/null) &&
    own_formula <(git -C "$tap_dir" show :Formula/agk.rb 2>/dev/null); then
    echo "release: discarding the unfinished Formula/agk.rb of a previous run" >&2
    git -C "$tap_dir" reset --quiet -- Formula/agk.rb
    git -C "$tap_dir" checkout --quiet -- Formula/agk.rb 2>/dev/null || rm -f "$tap_dir/Formula/agk.rb"
    dirty="$(git -C "$tap_dir" status --porcelain --untracked-files=all)"
  fi
  [[ -z "$dirty" ]] || die "tap checkout has local changes"
}

# own_formula FILE: FILE is this version's render (any sha256) or the tap's committed formula.
own_formula() {
  local content
  content="$(cat "$1")"
  [[ "$content" == "$(git -C "$tap_dir" show HEAD:Formula/agk.rb 2>/dev/null)" ]] && return 0
  [[ "$(sed -E 's/^  sha256 "[0-9a-f]{64}"$/  sha256 "{{SHA256}}"/' <<<"$content")" == \
    "$(sed -e "s/{{VERSION}}/$version/g" "$template")" ]]
}

tag_exists=""
if [[ -z "$dry_run" ]]; then
  [[ "$(git branch --show-current)" == "main" ]] || die "not on main"
  [[ -z "$(git status --porcelain --untracked-files=all)" ]] || die "working tree is not clean"
  git fetch --quiet origin main --tags
  [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main is not in sync with origin/main"
  if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
    [[ "$(git rev-list -n 1 "$tag")" == "$(git rev-parse HEAD)" ]] ||
      die "tag $tag exists on another commit; pick a new version"
    tag_exists=1
  fi
  check_tap
  git -C "$tap_dir" pull --quiet --ff-only origin "$tap_branch" || die "cannot fast-forward the tap checkout"
fi

go vet ./...
go test ./...

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
tarball="$tmp/agk.tar.gz"

if [[ -n "$dry_run" ]]; then
  # Local archive of HEAD: its sha differs from GitHub's tarball, so it only shows the shape.
  git archive --format=tar.gz --prefix="agterm-kit-$version/" -o "$tarball" HEAD
else
  [[ -n "$tag_exists" ]] || git tag -a "$tag" -m "agk $version"
  git push --quiet origin "refs/tags/$tag"
  url="https://github.com/$repo/archive/refs/tags/$tag.tar.gz"
  curl -fsSL --retry 5 --retry-all-errors -o "$tarball" "$url" || die "download $url failed; rerun to retry"
fi
sha="$(shasum -a 256 "$tarball" | cut -d' ' -f1)"

formula="$tmp/agk.rb"
sed -e "s/{{VERSION}}/$version/g" -e "s/{{SHA256}}/$sha/g" "$template" >"$formula"
! grep -q '{{' "$formula" || die "unrendered placeholder in $template"

if [[ -n "$dry_run" ]]; then
  cat "$formula"
  if [[ -d "$tap_dir/.git" ]]; then
    echo "release: tap checkout found at $tap_dir" >&2
  else
    echo "release: warning: no tap checkout at $tap_dir (set AGK_TAP_DIR)" >&2
  fi
  echo "release: dry run, nothing tagged or pushed" >&2
  exit 0
fi

mkdir -p "$tap_dir/Formula"
cp "$formula" "$tap_dir/Formula/agk.rb"
if [[ -z "$(git -C "$tap_dir" status --porcelain --untracked-files=all)" ]]; then
  echo "release: tap already has agk $version"
else
  git -C "$tap_dir" add Formula/agk.rb
  git -C "$tap_dir" commit --quiet -m "agk $version"
fi
git -C "$tap_dir" push --quiet origin "HEAD:refs/heads/$tap_branch"

echo "release: agk $version tagged and published to the tap"
echo "release: brew update && brew upgrade agk && agk setup"
