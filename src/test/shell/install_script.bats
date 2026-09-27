#!/usr/bin/env bats
# SPDX-License-Identifier: Apache-2.0
#
# install.sh installs labctl and the lab content from one release archive, with
# no sudo, and upgrades in place without losing the user's .env or lab state.
# Releases are served from a local directory through file:// URLs.

setup() {
  ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../../.." && pwd)"
  WORK="$(mktemp -d "${TMPDIR:-/tmp}/snowops-install.XXXXXX")"
  export HOME="$WORK/home"
  mkdir -p "$HOME"
  export SNOWOPS_RELEASE_URL="file://$WORK/releases"
  export SNOWOPS_VERSION=9.9.9
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
  ARCHIVE="labctl_9.9.9_${OS}_${ARCH}.tar.gz"
}

teardown() {
  rm -rf "$WORK"
}

# release <marker> [no-content] builds a release whose files contain marker.
release() {
  local stage="$WORK/stage" dir="$WORK/releases/download/v9.9.9"
  rm -rf "$stage" && mkdir -p "$stage/content/scenarios/demo" "$stage/content/config" "$dir"
  printf '#!/bin/sh\necho labctl %s\n' "$1" >"$stage/labctl"
  chmod +x "$stage/labctl"
  echo "$1" >"$stage/content/scenarios/demo/scenario.yaml"
  echo "PROFILE=k3d" >"$stage/content/config/.env.example"
  [ "${2:-}" = no-content ] && rm -rf "$stage/content"
  tar -czf "$dir/$ARCHIVE" -C "$stage" .
  (cd "$dir" && { command -v sha256sum >/dev/null && sha256sum "$ARCHIVE" || shasum -a 256 "$ARCHIVE"; } >checksums.txt)
}

@test "installs labctl and the lab content without sudo" {
  release v1
  run sh "$ROOT/install.sh"
  [ "$status" -eq 0 ]
  [ "$("$HOME/.local/bin/labctl")" = "labctl v1" ]
  [ "$(cat "$HOME/.snowops/lab/scenarios/demo/scenario.yaml")" = "v1" ]
  [ "$(cat "$HOME/.snowops/lab/VERSION")" = "9.9.9" ]
  [ -f "$HOME/.snowops/lab/.env" ]
  [[ "$output" == *"labctl init"* ]]
}

@test "an upgrade replaces the content and keeps .env and lab state" {
  release v1
  sh "$ROOT/install.sh" >/dev/null
  echo "CLUSTER_NAME=mine" >"$HOME/.snowops/lab/.env"
  mkdir -p "$HOME/.snowops/lab/.labctl" && echo kept >"$HOME/.snowops/lab/.labctl/history"
  echo stale >"$HOME/.snowops/lab/scenarios/removed-upstream"

  release v2
  run sh "$ROOT/install.sh"
  [ "$status" -eq 0 ]
  [ "$(cat "$HOME/.snowops/lab/scenarios/demo/scenario.yaml")" = "v2" ]
  [ ! -e "$HOME/.snowops/lab/scenarios/removed-upstream" ]
  [ "$(cat "$HOME/.snowops/lab/.env")" = "CLUSTER_NAME=mine" ]
  [ "$(cat "$HOME/.snowops/lab/.labctl/history")" = "kept" ]
}

@test "a corrupt download is refused and the current install is untouched" {
  release v1
  sh "$ROOT/install.sh" >/dev/null
  release v2
  echo "0000000000000000000000000000000000000000000000000000000000000000  $ARCHIVE" \
    >"$WORK/releases/download/v9.9.9/checksums.txt"
  run sh "$ROOT/install.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"checksum mismatch"* ]]
  [ "$("$HOME/.local/bin/labctl")" = "labctl v1" ]
}

@test "an archive without lab content is refused" {
  release v1 no-content
  run sh "$ROOT/install.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no lab content"* ]]
}

@test "a missing release names the URL it tried" {
  run env SNOWOPS_VERSION=0.0.1 sh "$ROOT/install.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"could not download"*"v0.0.1"* ]]
}

@test "says how to put labctl on PATH when it is not" {
  release v1
  run env PATH="/usr/bin:/bin:/usr/sbin:/sbin" sh "$ROOT/install.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *"is not on your PATH"* ]]
}
