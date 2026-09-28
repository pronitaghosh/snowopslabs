#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Install or upgrade SnowOps Labs: the labctl binary and the lab content it
# runs (scenarios, platform components, runtime scripts), from one release.
#
#   curl -fsSL https://raw.githubusercontent.com/sagar2395/snowopslabs/main/install.sh | sh
#
# Runs on macOS, Linux and Windows under WSL2, and needs no sudo:
#   labctl          -> $SNOWOPS_BIN_DIR   (default ~/.local/bin)
#   lab content     -> $SNOWOPS_HOME/lab  (default ~/.snowops/lab)
# Re-running it upgrades both and keeps the lab's .env and its .labctl state.
#
# Environment:
#   SNOWOPS_VERSION      version to install, e.g. 1.5.0 (default: the latest release)
#   SNOWOPS_BIN_DIR      where labctl goes
#   SNOWOPS_HOME         labctl's state directory
#   SNOWOPS_RELEASE_URL  releases base URL (default: the GitHub releases page)

set -eu

REPO="sagar2395/snowopslabs"
BIN_DIR="${SNOWOPS_BIN_DIR:-$HOME/.local/bin}"
LAB_DIR="${SNOWOPS_HOME:-$HOME/.snowops}/lab"
RELEASE_URL="${SNOWOPS_RELEASE_URL:-https://github.com/$REPO/releases}"

say() { printf '%s\n' "$*"; }
fail() {
  printf 'install.sh: %s\n' "$*" >&2
  exit 1
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo darwin ;;
    Linux) echo linux ;;
    MINGW* | MSYS* | CYGWIN*) fail "run this inside WSL2 (Ubuntu), not in Windows directly" ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo amd64 ;;
    arm64 | aarch64) echo arm64 ;;
    *) fail "unsupported CPU architecture: $(uname -m)" ;;
  esac
}

# latest_version reads the version the releases page's "latest" link redirects to.
latest_version() {
  url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$RELEASE_URL/latest")" ||
    fail "could not reach $RELEASE_URL (check your connection, or set SNOWOPS_VERSION)"
  version="${url##*/}"
  version="${version#v}"
  case "$version" in
    '' | latest) fail "could not work out the latest version; set SNOWOPS_VERSION" ;;
  esac
  echo "$version"
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

# install_content replaces each content directory in LAB_DIR with the release's
# copy, leaving everything else there (.env, .labctl) untouched.
install_content() {
  src="$1"
  mkdir -p "$LAB_DIR"
  for entry in "$src"/*; do
    [ -e "$entry" ] || continue
    name="$(basename "$entry")"
    rm -rf "${LAB_DIR:?}/$name"
    mv "$entry" "$LAB_DIR/$name"
  done
}

main() {
  command -v curl >/dev/null 2>&1 || fail "curl is required"
  command -v tar >/dev/null 2>&1 || fail "tar is required"

  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${SNOWOPS_VERSION:-$(latest_version)}"
  version="${version#v}"
  archive="labctl_${version}_${os}_${arch}.tar.gz"
  base="$RELEASE_URL/download/v$version"

  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT INT TERM

  say "Downloading SnowOps Labs $version for $os/$arch..."
  curl -fsSL "$base/$archive" -o "$tmp/$archive" || fail "could not download $base/$archive"
  curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "could not download the checksums"

  expected="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
  [ -n "$expected" ] || fail "$archive is not listed in checksums.txt"
  [ "$(sha256 "$tmp/$archive")" = "$expected" ] || fail "checksum mismatch for $archive; the download is corrupt"

  mkdir -p "$tmp/x"
  tar -xzf "$tmp/$archive" -C "$tmp/x"
  [ -f "$tmp/x/labctl" ] || fail "$archive has no labctl binary"
  [ -d "$tmp/x/content" ] || fail "$archive predates bundled lab content; install a newer release"

  mkdir -p "$BIN_DIR"
  # mv replaces the file rather than rewriting it in place, which macOS would
  # treat as a tampered signed binary and kill on its next run.
  mv "$tmp/x/labctl" "$BIN_DIR/labctl"
  chmod 755 "$BIN_DIR/labctl"
  install_content "$tmp/x/content"
  echo "$version" >"$LAB_DIR/VERSION"
  if [ ! -f "$LAB_DIR/.env" ] && [ -f "$LAB_DIR/config/.env.example" ]; then
    cp "$LAB_DIR/config/.env.example" "$LAB_DIR/.env"
  fi

  say ""
  say "Installed labctl $version to $BIN_DIR/labctl and the lab to $LAB_DIR."
  case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
      say ""
      say "$BIN_DIR is not on your PATH. Add this line to ~/.bashrc or ~/.zshrc, then open a new terminal:"
      say "  export PATH=\"$BIN_DIR:\$PATH\""
      ;;
  esac
  say ""
  say "Next:"
  say "  labctl init    # build the lab: tools, Docker, cluster, platform (about 5-10 minutes)"
  say "  labctl ui      # open the dashboard at http://localhost:3939"
}

main "$@"
