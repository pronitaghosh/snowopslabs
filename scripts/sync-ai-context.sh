#!/bin/sh
# Keeps Claude Code and OpenCode reading the same context.
# Project files are already shared (CLAUDE.md is canonical, OpenCode falls
# back to it, and both tools read .claude/skills/), so this script only syncs
# the global files that live outside the repo. Newest file wins.
# Usage: scripts/sync-ai-context.sh [--check]
#   --check  report drift and exit 1 instead of copying anything.
set -eu

root=$(
  unset CDPATH
  cd -- "$(dirname -- "$0")/.." && pwd
)
cd "$root"

mode=sync
if [ "${1:-}" = "--check" ]; then
  mode=check
fi

drift=0

sync_pair() {
  # Copies the newer file over the older one, or reports drift in check mode.
  src=$1
  dst=$2
  label=$3
  if [ ! -f "$src" ] && [ ! -f "$dst" ]; then
    echo "skip: $label (neither side exists yet)"
    return 0
  fi
  if [ ! -f "$src" ] || [ ! -f "$dst" ]; then
    echo "drift: $label (only one side exists)"
    drift=1
    return 0
  fi
  if cmp -s "$src" "$dst"; then
    echo "ok: $label"
    return 0
  fi
  if [ "$mode" = "check" ]; then
    echo "drift: $label"
    drift=1
    return 0
  fi
  if [ "$src" -nt "$dst" ]; then
    cp "$src" "$dst"
    echo "synced: $label ($src -> $dst)"
  else
    cp "$dst" "$src"
    echo "synced: $label ($dst -> $src)"
  fi
}

global_claude="$HOME/.claude/CLAUDE.md"
global_opencode="$HOME/.config/opencode/AGENTS.md"
claude_mcp="$HOME/.claude/mcp.json"
opencode_cfg="$HOME/.config/opencode/opencode.json"

sync_pair "$global_claude" "$global_opencode" "global context"

# MCP servers need a format conversion, so only report drift here.
if [ -f "$claude_mcp" ] && [ -f "$opencode_cfg" ]; then
  echo "ok: MCP configs both exist (convert mcpServers <-> .mcp by hand)"
elif [ -f "$claude_mcp" ] || [ -f "$opencode_cfg" ]; then
  echo "drift: MCP config (only one side exists)"
  drift=1
else
  echo "skip: MCP config (neither side exists yet)"
fi

# Project files need no syncing; verify both tools see the same context.
if [ -f "$root/CLAUDE.md" ] && [ -f "$root/opencode.json" ]; then
  echo "ok: project context (CLAUDE.md + opencode.json instructions)"
else
  echo "drift: project context (CLAUDE.md or opencode.json missing)"
  drift=1
fi
if [ -d "$root/.claude/skills" ]; then
  echo "ok: shared skills (.claude/skills/, read by both tools)"
else
  echo "drift: shared skills (.claude/skills/ missing)"
  drift=1
fi

if [ "$mode" = "check" ]; then
  exit "$drift"
fi
