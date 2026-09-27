#!/usr/bin/env bash
set -euo pipefail

# Copy the lab content labctl runs at runtime into <dest>, keeping the
# repository layout so every relative path in the scripts still resolves.
# goreleaser runs it before archiving; the release installer unpacks the result
# into ~/.snowops/lab.
#
# Usage: scripts/stage-release-content.sh <dest>
#
# Only committed files are staged (git archive), with their executable bits and
# the LF line endings .gitattributes enforces.

DEST="${1:?usage: stage-release-content.sh <dest>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# The content labctl reads or runs; keep in step with what scripts reference.
CONTENT="scenarios incidents learn challenges platform runtimes bootstrap apps config src/engine src/services"

rm -rf "$DEST"
mkdir -p "$DEST"
# shellcheck disable=SC2086 # one path per word
git -C "$ROOT" archive --format=tar HEAD $CONTENT | tar -x -C "$DEST"
echo "Staged lab content into $DEST"
