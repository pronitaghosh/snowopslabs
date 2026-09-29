#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Opens the pull request that releases the lab as vX.Y.Z. Its branch holds two
# commits off main: the first sets LAB_VERSION to the release, the second moves
# it on to the next development version, so main is never left on a release.
# When the pull request merges, the Tag release workflow tags the first commit.
#
# Usage: scripts/release.sh X.Y.Z [NEXT]
#   NEXT  the version main moves to afterwards (default X.<Y+1>.0-dev)
#
# Environment:
#   RELEASE_REMOTE  the remote that holds main (default origin)
set -eu

fail() {
  echo "release: $*" >&2
  exit 1
}

[ $# -ge 1 ] && [ $# -le 2 ] || {
  echo "usage: scripts/release.sh X.Y.Z [NEXT]" >&2
  exit 2
}
version=$1
remote=${RELEASE_REMOTE:-origin}

printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' ||
  fail "the version must be X.Y.Z with no leading v (got '$version')"
if [ $# -eq 2 ]; then
  next=$2
else
  major=${version%%.*}
  rest=${version#*.}
  next="$major.$((${rest%%.*} + 1)).0-dev"
fi
printf '%s\n' "$next" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-dev$' ||
  fail "the next version must be X.Y.Z-dev (got '$next')"

root=$(
  unset CDPATH
  cd -- "$(dirname -- "$0")/.." && pwd
)
cd "$root"

command -v gh >/dev/null 2>&1 ||
  fail "gh not found; install the GitHub CLI and run 'gh auth login'"
[ -z "$(git status --porcelain)" ] ||
  fail "the working tree has changes; commit or stash them, then run this again"

git fetch --quiet --tags "$remote" main
if git rev-parse -q --verify "refs/tags/v$version" >/dev/null; then
  fail "v$version is already tagged; release the next version instead"
fi
current=$(git show "$remote/main:LAB_VERSION" | tr -d '[:space:]')
case $current in
  *-dev) ;;
  *) fail "$remote/main has LAB_VERSION $current, not a -dev version; merge the open release pull request or set main back to -dev first" ;;
esac

branch="release/v$version"
start=$(git symbolic-ref -q --short HEAD || git rev-parse HEAD)
git switch --quiet -c "$branch" "$remote/main" ||
  fail "could not create $branch; delete the branch left by an earlier attempt (git branch -D $branch)"
printf '%s\n' "$version" >LAB_VERSION
git commit --quiet -s -m "release: v$version" LAB_VERSION
printf '%s\n' "$next" >LAB_VERSION
git commit --quiet -s -m "chore: start $next" LAB_VERSION
git push --quiet -u "$remote" "$branch"
git checkout --quiet "$start"

gh pr create --base main --head "$branch" --title "release: v$version" --body "Releases v$version and moves main on to $next.

Merge with **Create a merge commit** or **Rebase and merge**, never squash: the
Tag release workflow tags the commit that sets LAB_VERSION to $version, and a
squash merge removes it. The tag then builds the draft GitHub Release.

See RELEASING.md."

echo "Opened the release pull request for v$version. Once CI is green, merge it"
echo "(merge commit or rebase, not squash); then review and publish the draft Release."
