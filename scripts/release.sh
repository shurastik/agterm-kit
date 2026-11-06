#!/usr/bin/env bash
# Release agk: tag vVERSION, then render the Homebrew formula into the tap and push it.
#
#   scripts/release.sh 0.1.0
#   DRY_RUN=1 scripts/release.sh 0.1.0   # gates + rendered formula, no tag, no push
#
# AGK_TAP_DIR is the shurastik/homebrew-tap checkout (default ~/Projects/homebrew-tap).
set -euo pipefail

repo="shurastik/agterm-kit"
tap_dir="${AGK_TAP_DIR:-$HOME/Projects/homebrew-tap}"
dry_run="${DRY_RUN:-}"

die() { echo "release: $*" >&2; exit 1; }

[[ $# -eq 1 ]] || die "usage: scripts/release.sh X.Y.Z"
version="$1"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must be X.Y.Z, got '$version'"
tag="v$version"

root="$(git rev-parse --show-toplevel)"
cd "$root"
template="packaging/homebrew/agk.rb.tmpl"

if [[ -z "$dry_run" ]]; then
  [[ "$(git branch --show-current)" == "main" ]] || die "not on main"
  [[ -z "$(git status --porcelain)" ]] || die "working tree is not clean"
  git fetch --quiet origin main --tags
  [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main is not in sync with origin/main"
  ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null || die "tag $tag already exists"
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
  git tag -a "$tag" -m "agk $version"
  git push origin "$tag"
  url="https://github.com/$repo/archive/refs/tags/$tag.tar.gz"
  curl -fsSL --retry 5 --retry-all-errors -o "$tarball" "$url" || die "download $url failed"
fi
sha="$(shasum -a 256 "$tarball" | cut -d' ' -f1)"

formula="$tmp/agk.rb"
sed -e "s/{{VERSION}}/$version/g" -e "s/{{SHA256}}/$sha/g" "$template" >"$formula"
! grep -q '{{' "$formula" || die "unrendered placeholder in $template"

if [[ -n "$dry_run" ]]; then
  cat "$formula"
  echo "release: dry run, nothing tagged or pushed" >&2
  exit 0
fi

[[ -d "$tap_dir/.git" ]] || die "tap checkout not found at $tap_dir (set AGK_TAP_DIR)"
[[ -z "$(git -C "$tap_dir" status --porcelain)" ]] || die "tap checkout has local changes"
git -C "$tap_dir" pull --quiet --ff-only
mkdir -p "$tap_dir/Formula"
cp "$formula" "$tap_dir/Formula/agk.rb"
git -C "$tap_dir" add Formula/agk.rb
git -C "$tap_dir" commit --quiet -m "agk $version"
git -C "$tap_dir" push --quiet

echo "release: agk $version tagged and published to the tap"
echo "release: brew update && brew upgrade agk && agk setup"
