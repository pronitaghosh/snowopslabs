#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Tags the release commit a push to main brought in. A release commit is one
# whose LAB_VERSION is X.Y.Z with no -dev and no vX.Y.Z tag yet; it gets an
# annotated vX.Y.Z tag and the tag name is printed. A squash-merged release
# pull request has lost that commit, so it fails with what to do instead.
#
# Usage: scripts/release-tag.sh BEFORE AFTER
#   BEFORE  the commit main was on before the push (all zeros for a new branch)
#   AFTER   the commit main is on now
set -eu

[ $# -eq 2 ] || {
  echo "usage: scripts/release-tag.sh BEFORE AFTER" >&2
  exit 2
}
range="$1..$2"
case $1 in *[!0]*) ;; *) range=$2 ;; esac

tag=""
missing=""
for c in $(git rev-list --reverse "$range"); do
  v=$(git show "$c:LAB_VERSION" 2>/dev/null | tr -d '[:space:]') || continue
  subject=$(git log -1 --format=%s "$c")
  if printf '%s\n' "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    git rev-parse -q --verify "refs/tags/v$v" >/dev/null && continue
    if [ -n "$tag" ]; then
      echo "release-tag: this push holds two releases ($tag and v$v); tag v$v by hand" >&2
      exit 1
    fi
    git tag -a "v$v" -m "v$v" "$c"
    tag="v$v"
  else
    case $subject in
      "release: v"*)
        missing=${subject#release: }
        missing=${missing%% *}
        ;;
    esac
  fi
done

if [ -z "$tag" ] && [ -n "$missing" ] &&
  ! git rev-parse -q --verify "refs/tags/$missing" >/dev/null; then
  echo "release-tag: $missing was merged without its release commit (a squash merge?)." >&2
  echo "Run 'make release VERSION=${missing#v}' again and merge that pull request with a merge commit or a rebase." >&2
  exit 1
fi
[ -z "$tag" ] || printf '%s\n' "$tag"
