#!/usr/bin/env bats
# SPDX-License-Identifier: Apache-2.0
#
# runtimes/_lib/docker.sh: how the lab starts Docker and pulls its images. The
# default colima VM (2 CPU / 2 GB) cannot hold the lab, so a stopped colima must
# be started at the lab's size and never shrunk; a stalled image pull must fail
# loudly instead of hanging `labctl init`.

load 'helpers/stub'

setup() {
  stub_setup
  ROOT="$(project_root)"
  STATE="$STUB_DIR/state"
  mkdir -p "$STATE"
  export STATE

  # docker answers `info` only once colima has "started"; `pull` obeys
  # $PULL_MODE (ok | stall | fail).
  cat >"$STUB_BIN/docker" <<'EOF'
#!/usr/bin/env bash
echo "docker $*" >>"$STATE/calls"
case "$1" in
  info) [ -f "$STATE/running" ] ;;
  image) exit 1 ;;
  port)
    [ -n "${LB_MISSING:-}" ] && { echo "Error: No such container: $2" >&2; exit 1; }
    case "$3" in
      80/tcp) printf '0.0.0.0:%s\n[::]:%s\n' "${LB_HTTP:-80}" "${LB_HTTP:-80}" ;;
      443/tcp) printf '0.0.0.0:%s\n' "${LB_HTTPS:-443}" ;;
    esac ;;
  pull)
    case "${PULL_MODE:-ok}" in
      stall) sleep 30 ;;
      fail) exit 1 ;;
    esac ;;
esac
EOF
  cat >"$STUB_BIN/colima" <<'EOF'
#!/usr/bin/env bash
echo "colima $*" >>"$STATE/calls"
case "$1" in
  start) touch "$STATE/running" ;;
  list) [ -f "$STATE/colima.json" ] && cat "$STATE/colima.json" ;;
esac
exit 0
EOF
  cat >"$STUB_BIN/uname" <<'EOF'
#!/usr/bin/env bash
echo "${FAKE_UNAME:-Darwin}"
EOF
  chmod +x "$STUB_BIN/docker" "$STUB_BIN/colima" "$STUB_BIN/uname"
}

teardown() {
  stub_teardown
}

lib() {
  # shellcheck disable=SC1091
  . "$ROOT/runtimes/_lib/docker.sh"
  "$@"
}

@test "a running daemon is left alone" {
  touch "$STATE/running"
  run lib ensure_docker_running
  [ "$status" -eq 0 ]
  ! grep -q "colima start" "$STATE/calls"
}

@test "a stopped colima starts at the lab size, not colima's 2 GB default" {
  run lib ensure_docker_running
  [ "$status" -eq 0 ]
  grep -q "colima start --cpu 2 --memory 4" "$STATE/calls"
}

@test "LAB_CPUS and LAB_MEMORY size the VM" {
  LAB_CPUS=4 LAB_MEMORY=6 run lib ensure_docker_running
  [ "$status" -eq 0 ]
  grep -q "colima start --cpu 4 --memory 6" "$STATE/calls"
}

@test "a bigger existing VM is never shrunk" {
  echo '{"name":"default","status":"Stopped","cpus":6,"memory":8589934592}' >"$STATE/colima.json"
  run lib ensure_docker_running
  [ "$status" -eq 0 ]
  grep -q "colima start --cpu 6 --memory 8" "$STATE/calls"
}

@test "a smaller existing VM is grown to the lab size" {
  echo '{"name":"default","status":"Stopped","cpus":2,"memory":2147483648}' >"$STATE/colima.json"
  run lib ensure_docker_running
  [ "$status" -eq 0 ]
  grep -q "colima start --cpu 2 --memory 4" "$STATE/calls"
}

@test "a stopped daemon on Linux fails with the start command" {
  FAKE_UNAME=Linux run lib ensure_docker_running
  [ "$status" -eq 1 ]
  [[ "$output" == *"sudo systemctl start docker"* ]]
  ! grep -q "colima" "$STATE/calls"
}

@test "a stopped daemon on WSL points at Docker Desktop's WSL Integration" {
  FAKE_UNAME=Linux WSL_DISTRO_NAME=Ubuntu run lib ensure_docker_running
  [ "$status" -eq 1 ]
  [[ "$output" == *"WSL Integration"* ]]
}

@test "a stalled pull times out instead of hanging" {
  PULL_MODE=stall run lib pull_with_timeout rancher/k3s:v1 5
  [ "$status" -eq 124 ]
}

@test "prepull retries once, then names the image and the next step" {
  PULL_MODE=fail run lib prepull_images rancher/k3s:v1
  [ "$status" -eq 1 ]
  [ "$(grep -c "docker pull rancher/k3s:v1" "$STATE/calls")" -eq 2 ]
  [[ "$output" == *"could not download rancher/k3s:v1"* ]]
  [[ "$output" == *"re-run 'labctl init'"* ]]
}

@test "prepull succeeds on a healthy network" {
  run lib prepull_images rancher/k3s:v1 ghcr.io/k3d-io/k3d-proxy:5.8.3
  [ "$status" -eq 0 ]
}

@test "the ingress ports a fallback cluster bound are recorded for labctl" {
  export SNOWOPS_HOME="$STUB_DIR/snowops"
  LB_HTTP=8080 LB_HTTPS=8443 run lib record_ingress_ports lab k3d-lab-serverlb
  [ "$status" -eq 0 ]
  [ "$(cat "$SNOWOPS_HOME/clusters/lab.env")" = "$(printf 'HTTP_PORT=8080\nHTTPS_PORT=8443')" ]
}

@test "forgetting a cluster removes its record" {
  export SNOWOPS_HOME="$STUB_DIR/snowops"
  mkdir -p "$SNOWOPS_HOME/clusters"
  echo "HTTP_PORT=8080" >"$SNOWOPS_HOME/clusters/lab.env"
  run lib forget_cluster lab
  [ "$status" -eq 0 ]
  [ ! -e "$SNOWOPS_HOME/clusters/lab.env" ]
}

@test "no mapped port writes no record" {
  export SNOWOPS_HOME="$STUB_DIR/snowops"
  LB_MISSING=1 run lib record_ingress_ports lab missing-container
  [ "$status" -eq 0 ]
  [ ! -e "$SNOWOPS_HOME/clusters/lab.env" ]
}
