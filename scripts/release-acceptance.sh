#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Release acceptance: walks the live lab on this machine the way a learner
# does (init, URLs, dashboards, UI, apps, scenarios, an incident) and writes a
# PASS/FAIL report. Every step runs even after one fails, and the lab is left
# as it was found. The journey is described in docs/release-acceptance.md.
#
# Usage: scripts/release-acceptance.sh [--quick] [--only step,step] [--list]
#   --quick   skip the hermetic gates (make test, lint, docs-check)
#   --only    run just the named steps
#   --list    print the steps and exit
#
# Environment:
#   RA_INCIDENT   fault for the incident loop (default crashloop-bad-config)
#   RA_SCENARIO   inactive scenario to cycle up -> verify -> down (default none)
#   RA_UI_PORT    port for the throwaway `labctl ui` (default 39399)
#   RA_OUT        report directory (default ~/.snowops/acceptance/<UTC time>)
#   LABCTL        labctl to test (default <clone>/bin/labctl, built by `build`)

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LABCTL="${LABCTL:-$ROOT/bin/labctl}"
RA_INCIDENT="${RA_INCIDENT:-crashloop-bad-config}"
RA_SCENARIO="${RA_SCENARIO:-}"
RA_UI_PORT="${RA_UI_PORT:-39399}"
RA_WAIT="${RA_WAIT:-180}"
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-admin}"
OUT="${RA_OUT:-${SNOWOPS_HOME:-$HOME/.snowops}/acceptance/$(date -u +%Y%m%dT%H%M%SZ)}"

ALL_STEPS="build gates content doctor init status urls datasources dashboards ui apps scenarios incident learning leaks"

# Step results: 0 pass, 1 fail, 2 skip.
SKIP=2

log() { printf '%s\n' "$*"; }

# ---------------------------------------------------------------- lab address

# cluster_name is the lab's cluster, from the environment, .env or the default.
cluster_name() {
  local name="${CLUSTER_NAME:-}"
  if [ -z "$name" ] && [ -f "$ROOT/.env" ]; then
    name="$(sed -n 's/^CLUSTER_NAME=//p' "$ROOT/.env" | tail -1)"
  fi
  printf '%s\n' "${name:-snowops}"
}

# record_value <key> reads the port or domain suffix the runtime recorded.
record_value() {
  local file
  file="${SNOWOPS_HOME:-$HOME/.snowops}/clusters/$(cluster_name).env"
  [ -f "$file" ] && sed -n "s/^$1=//p" "$file" | tail -1
}

# lab_url <service> prints the URL a learner opens for <service>.
lab_url() {
  local suffix port
  suffix="$(record_value DOMAIN_SUFFIX)"
  port="$(record_value HTTP_PORT)"
  suffix="${suffix:-$(cluster_name).localhost}"
  if [ -z "$port" ] || [ "$port" = 80 ]; then
    printf 'http://%s.%s\n' "$1" "$suffix"
  else
    printf 'http://%s.%s:%s\n' "$1" "$suffix" "$port"
  fi
}

# grafana_get <path> calls Grafana's API as the admin user.
grafana_get() {
  curl -fsS -m 20 -u "admin:$GRAFANA_ADMIN_PASSWORD" "$(lab_url grafana)$1"
}

# wait_until <seconds> <command...> retries the command every 5 s.
wait_until() {
  local deadline=$(($(date +%s) + $1))
  shift
  until "$@"; do
    [ "$(date +%s)" -ge "$deadline" ] && return 1
    sleep 5
  done
}

# ---------------------------------------------------------------------- steps

step_build() {
  (cd "$ROOT" && make cli-build) || return 1
  local onpath
  onpath="$(command -v labctl || true)"
  if [ -n "$onpath" ] && ! cmp -s "$onpath" "$LABCTL"; then
    log "WARNING: 'labctl' on PATH ($onpath) is not this build; learners following the docs run that one."
    log "         Run 'make cli-install' to replace it."
  fi
  "$LABCTL" --version
}

step_gates() {
  (cd "$ROOT" && make test && make lint && make docs-check)
}

step_content() { "$LABCTL" validate; }

step_doctor() {
  local out
  out="$("$LABCTL" doctor 2>&1)"
  log "$out"
  case "$out" in *"Ready to run"*) ;; *) return 1 ;; esac
}

step_init() {
  local out
  out="$("$LABCTL" init 2>&1)"
  log "$out"
  case "$out" in *"=== Lab is up ==="*) ;; *) return 1 ;; esac
}

step_status() {
  local out
  out="$("$LABCTL" status 2>&1)" || {
    log "$out"
    return 1
  }
  log "$out"
  case "$out" in *unreachable* | *"not running"*) return 1 ;; esac
}

step_urls() {
  local rc=0 body
  body="$(curl -fsS -m 10 "$(lab_url grafana)/api/health")" || rc=1
  log "grafana /api/health: $body"
  case "$body" in *'"database"'*) ;; *) rc=1 ;; esac
  if curl -fsS -m 10 -o /dev/null "$(lab_url prometheus)/-/ready"; then
    log "prometheus /-/ready: ok"
  else
    log "prometheus /-/ready: FAILED"
    rc=1
  fi
  return "$rc"
}

step_datasources() {
  local uids uid health rc=0
  uids="$(grafana_get /api/datasources | jq -r '.[].uid')" || return 1
  [ -n "$uids" ] || {
    log "Grafana has no datasources"
    return 1
  }
  for uid in $uids; do
    health="$(curl -sS -m 30 -u "admin:$GRAFANA_ADMIN_PASSWORD" \
      "$(lab_url grafana)/api/datasources/uid/$uid/health")"
    log "$uid: $health"
    [ "$(printf '%s' "$health" | jq -r '.status // empty' 2>/dev/null)" = OK ] || rc=1
  done
  return "$rc"
}

step_dashboards() {
  local count up namespaces
  count="$(grafana_get '/api/search?type=dash-db' | jq 'length')" || return 1
  log "dashboards: $count"
  up="$(grafana_get '/api/datasources/proxy/uid/prometheus/api/v1/query?query=count(up%3D%3D1)' |
    jq -r '.data.result[0].value[1] // 0')" || return 1
  log "targets up: $up"
  namespaces="$(grafana_get '/api/datasources/proxy/uid/prometheus/api/v1/label/namespace/values' |
    jq '.data | length')" || return 1
  log "namespaces with metrics: $namespaces"
  [ "$count" -gt 0 ] && [ "$up" -gt 0 ] && [ "$namespaces" -gt 0 ]
}

UI_PID=""
stop_ui() {
  [ -n "$UI_PID" ] && kill "$UI_PID" 2>/dev/null
  UI_PID=""
}

step_ui() {
  local base="http://127.0.0.1:$RA_UI_PORT" rc=0 path
  "$LABCTL" ui --port "$RA_UI_PORT" >"$OUT/ui-server.log" 2>&1 &
  UI_PID=$!
  wait_until 30 curl -fsS -m 2 -o /dev/null "$base/" || {
    log "labctl ui did not answer on $base"
    stop_ui
    return 1
  }
  case "$(curl -fsS -m 10 "$base/")" in *'<div id="root">'*) log "/: SPA shell" ;; *) rc=1 ;; esac
  for path in scenarios incidents challenges 'runs?limit=1' apps status; do
    if curl -fsS -m 60 "$base/api/v2/$path" | jq -e . >/dev/null 2>&1; then
      log "/api/v2/$path: JSON"
    else
      log "/api/v2/$path: FAILED"
      rc=1
    fi
  done
  stop_ui
  return "$rc"
}

step_apps() {
  local out rc=0 name replicas ready
  out="$("$LABCTL" status 2>&1)" || return 1
  # Lines look like: "  go-api   replicas=1 ready=1".
  while read -r name replicas ready; do
    replicas="${replicas#replicas=}"
    ready="${ready#ready=}"
    log "$name: $ready/$replicas ready"
    [ "$replicas" = "$ready" ] || rc=1
  done <<EOF
$(printf '%s\n' "$out" | grep 'replicas=[0-9]* ready=')
EOF
  return "$rc"
}

# verify_runs <scenario> passes when verify grades the scenario, whether or not
# its objectives are complete: a fresh activation is expected to be pending.
verify_runs() {
  local out
  out="$("$LABCTL" scenario verify "$1" 2>&1)"
  log "$out"
  case "$out" in
    *panic* | *"unknown scenario"* | *"not active"*) return 1 ;;
    *✓* | *✗* | *pending* | *passed* | *PASS* | *FAIL*) return 0 ;;
  esac
  return 1
}

step_scenarios() {
  local active rc=0 name
  active="$("$LABCTL" scenario list 2>/dev/null | awk '$NF == "active" {print $1}')"
  for name in $active; do
    log "--- verify $name"
    verify_runs "$name" || rc=1
  done
  if [ -n "$RA_SCENARIO" ]; then
    case " $active " in
      *" $RA_SCENARIO "*) log "$RA_SCENARIO is already active; not cycling it" ;;
      *)
        log "--- cycle $RA_SCENARIO"
        "$LABCTL" scenario up "$RA_SCENARIO" || rc=1
        verify_runs "$RA_SCENARIO" || rc=1
        "$LABCTL" scenario down "$RA_SCENARIO" || rc=1
        ;;
    esac
  fi
  [ -n "$active$RA_SCENARIO" ] || return "$SKIP"
  return "$rc"
}

incident_detected() {
  "$LABCTL" incident status 2>&1 | tee -a "$OUT/incident-status.log" | grep -q "NOT RESOLVED"
}

incident_cleared() {
  "$LABCTL" incident status 2>&1 | grep -q "No incident is active"
}

apps_ready() { step_apps >/dev/null; }

step_incident() {
  local rc=0
  if ! incident_cleared; then
    log "An incident is already active; not injecting another."
    return "$SKIP"
  fi
  "$LABCTL" incident inject "$RA_INCIDENT" || return 1
  if wait_until "$RA_WAIT" incident_detected; then
    log "detection check fires"
  else
    log "the detection check never fired within ${RA_WAIT}s"
    rc=1
  fi
  "$LABCTL" incident resolve || rc=1
  if incident_cleared; then log "incident cleared"; else rc=1; fi
  if wait_until "$RA_WAIT" apps_ready; then
    log "every app is ready again"
  else
    log "apps did not recover within ${RA_WAIT}s"
    rc=1
  fi
  return "$rc"
}

step_learning() {
  local first
  "$LABCTL" learn list || return 1
  "$LABCTL" challenge list || return 1
  first="$("$LABCTL" challenge list 2>/dev/null | awk 'NR > 2 && NF {print $1; exit}')"
  [ -n "$first" ] && "$LABCTL" challenge info "$first" >/dev/null
}

namespaces() { kubectl get ns -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | sort; }

step_leaks() {
  local extra
  [ -f "$OUT/namespaces.before" ] || return "$SKIP"
  extra="$(namespaces | comm -13 "$OUT/namespaces.before" -)"
  if [ -n "$extra" ]; then
    log "namespaces left behind by the run:"
    log "$extra"
    return 1
  fi
  log "no namespaces left behind"
}

# ------------------------------------------------------------------- harness

# run_step <name> runs step_<name>, logs it to <name>.log and records the result.
run_step() {
  local name="$1" start rc result
  start="$(date +%s)"
  printf '%-12s ' "$name"
  "step_$name" >"$OUT/$name.log" 2>&1
  rc=$?
  case "$rc" in
    0) result=PASS ;;
    "$SKIP") result=SKIP ;;
    *) result=FAIL ;;
  esac
  printf '%s\t%s\t%ss\n' "$name" "$result" "$(($(date +%s) - start))" >>"$OUT/results.tsv"
  printf '%s (%ss)\n' "$result" "$(($(date +%s) - start))"
}

write_report() {
  {
    printf '# Release acceptance\n\n'
    printf -- '- Commit: `%s` on `%s`\n' "$(git -C "$ROOT" rev-parse --short HEAD)" \
      "$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    printf -- '- labctl: `%s`\n' "$("$LABCTL" --version 2>/dev/null)"
    printf -- '- Run: %s\n\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf '| Step | Result | Time | Log |\n|---|---|---|---|\n'
    while IFS="$(printf '\t')" read -r name result took; do
      printf '| %s | %s | %s | `%s.log` |\n' "$name" "$result" "$took" "$name"
    done <"$OUT/results.tsv"
  } >"$OUT/report.md"
}

main() {
  local steps="$ALL_STEPS" step
  while [ $# -gt 0 ]; do
    case "$1" in
      --quick) steps="$(printf '%s\n' $ALL_STEPS | grep -vx gates | tr '\n' ' ')" ;;
      --only)
        steps="$(printf '%s' "${2:-}" | tr ',' ' ')"
        shift
        ;;
      --list)
        printf '%s\n' $ALL_STEPS
        return 0
        ;;
      *)
        log "unknown argument: $1 (see the usage at the top of $0)"
        return 2
        ;;
    esac
    shift
  done
  for step in $steps; do
    case " $ALL_STEPS " in *" $step "*) ;; *)
      log "unknown step: $step (--list shows them)"
      return 2
      ;;
    esac
  done

  mkdir -p "$OUT"
  : >"$OUT/results.tsv"
  trap stop_ui EXIT
  namespaces >"$OUT/namespaces.before" 2>/dev/null || rm -f "$OUT/namespaces.before"
  log "Release acceptance -> $OUT"
  for step in $steps; do
    run_step "$step"
  done
  write_report
  log "Report: $OUT/report.md"
  ! grep -q "$(printf '\t')FAIL$(printf '\t')" "$OUT/results.tsv"
}

main "$@"
