#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Docker engine helpers shared by bootstrap/setup-tools.sh and the local
# runtimes (k3d, kind). Source it; it defines functions only.
#
# Sizing contract: a stopped colima is started at LAB_CPUS/LAB_MEMORY (default
# 2 CPU / 4 GB), never smaller than it already is. A running engine is never
# resized here — labctl's preflight refuses an undersized one and prints how.

LAB_CPUS="${LAB_CPUS:-2}"
LAB_MEMORY="${LAB_MEMORY:-4}"

# wait_for_docker <seconds> — succeed once the daemon answers.
wait_for_docker() {
  local limit="${1:-60}" waited=0
  until docker info >/dev/null 2>&1; do
    if [ "$waited" -ge "$limit" ]; then
      echo "ERROR: the Docker daemon did not answer within ${limit}s." >&2
      return 1
    fi
    sleep 2
    waited=$((waited + 2))
  done
}

# colima_size_arg <field> <wanted> — the larger of colima's configured value
# for field (cpus | memory, memory in GiB) and wanted, so a stopped VM is only
# ever grown. Falls back to wanted when colima reports nothing.
colima_size_arg() {
  local field="$1" wanted="$2" have=""
  if command -v jq >/dev/null 2>&1; then
    have="$(colima list --json 2>/dev/null | jq -r --arg f "$field" \
      'select(.name == "default") | .[$f] // empty' 2>/dev/null | head -n 1)"
  fi
  case "$have" in
    '' | *[!0-9]*) printf '%s' "$wanted" ;;
    *)
      # colima reports memory in bytes.
      [ "$field" = memory ] && have=$((have / 1073741824))
      if [ "$have" -gt "${wanted%%.*}" ]; then printf '%s' "$have"; else printf '%s' "$wanted"; fi
      ;;
  esac
}

# docker_access_problem prints why a present docker CLI cannot use the daemon
# when the cause is not "it is stopped", and fails; it succeeds (prints
# nothing) otherwise. Two cases make every later step fail confusingly:
#   - Docker Desktop's WSL shim, when WSL Integration is off for this distro
#   - a user just added to the docker group whose session predates it
docker_access_problem() {
  local out
  command -v docker >/dev/null 2>&1 || return 0
  out="$(docker info 2>&1 >/dev/null || true)"
  case "$out" in
    *"could not be found in this WSL 2 distro"*)
      echo "Docker Desktop is installed on Windows but not enabled for this WSL distro."
      echo "  Docker Desktop → Settings → Resources → WSL Integration → enable '${WSL_DISTRO_NAME:-this distro}',"
      echo "  Apply & Restart, then reopen this terminal and re-run 'labctl init'."
      return 1
      ;;
    *"permission denied"*)
      echo "Docker is running, but your user may not use it yet."
      if id -nG 2>/dev/null | grep -qw docker; then
        echo "  You are in the docker group, but this terminal started before you were added."
      else
        echo "  Add yourself to the docker group: sudo usermod -aG docker \"\$USER\""
      fi
      if [ -n "${WSL_DISTRO_NAME:-}" ]; then
        echo "  Then run 'wsl --shutdown' in PowerShell, reopen this terminal and re-run 'labctl init'."
      else
        echo "  Then log out and back in (or run 'newgrp docker') and re-run 'labctl init'."
      fi
      return 1
      ;;
  esac
  return 0
}

# ensure_docker_running — make the daemon answer, starting colima on macOS.
ensure_docker_running() {
  if docker info >/dev/null 2>&1; then
    return 0
  fi
  local problem
  if ! problem="$(docker_access_problem)"; then
    echo "ERROR: ${problem}" >&2
    return 1
  fi
  if [ "$(uname -s)" = "Darwin" ] && command -v colima >/dev/null 2>&1; then
    local cpus mem
    cpus="$(colima_size_arg cpus "$LAB_CPUS")"
    mem="$(colima_size_arg memory "$LAB_MEMORY")"
    echo "Starting colima with ${cpus} CPU / ${mem} GB (the first start downloads a VM image, ~1-2 min)..."
    colima start --cpu "$cpus" --memory "$mem" || return 1
    wait_for_docker 90 || return 1
    echo "Colima is running."
    return 0
  fi
  echo "ERROR: the Docker daemon is not running." >&2
  if [ "$(uname -s)" = "Darwin" ]; then
    echo "  Start Docker Desktop (open -a Docker), or install colima: brew install colima docker" >&2
  elif [ -n "${WSL_DISTRO_NAME:-}" ]; then
    echo "  Start Docker Desktop on Windows and enable Settings → Resources → WSL Integration" >&2
    echo "  for this distro, or start a native daemon: sudo service docker start" >&2
  else
    echo "  sudo systemctl start docker" >&2
  fi
  return 1
}

# pull_with_timeout <image> <seconds> — pull an image, giving up if it takes
# longer than seconds. A stalled registry download otherwise hangs cluster
# creation forever with no output. Returns 124 on timeout.
pull_with_timeout() {
  local image="$1" limit="$2" pid waited=0
  docker image inspect "$image" >/dev/null 2>&1 && return 0
  docker pull "$image" &
  pid=$!
  while kill -0 "$pid" 2>/dev/null; do
    if [ "$waited" -ge "$limit" ]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
      return 124
    fi
    sleep 5
    waited=$((waited + 5))
  done
  wait "$pid"
}

# prepull_images <image>... — pull each image with one retry, so a slow or
# stalled network surfaces as a clear error instead of a silent hang.
prepull_images() {
  local image rc
  for image in "$@"; do
    [ -n "$image" ] || continue
    rc=0
    pull_with_timeout "$image" 600 || rc=$?
    if [ "$rc" -ne 0 ]; then
      echo "Pull of ${image} failed or stalled — retrying once..." >&2
      rc=0
      pull_with_timeout "$image" 600 || rc=$?
    fi
    if [ "$rc" -ne 0 ]; then
      echo "ERROR: could not download ${image} (network stalled or registry unreachable)." >&2
      echo "  Check your connection (and any proxy/VPN), then re-run 'labctl init'." >&2
      return 1
    fi
  done
}
