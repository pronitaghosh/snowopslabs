#!/usr/bin/env bash
set -euo pipefail

# Measure the memory each scenario adds to a running lab and print the values
# for config/footprints.yaml. Run it from the repository root against a lab
# built with `labctl init` and nothing else active; it activates each scenario
# in turn, samples memory once it settles, and deactivates it again.
#
# Usage: scripts/measure-footprints.sh [scenario...]   (default: every scenario)
#
# Config (env):
#   LABCTL         labctl binary to use (default: labctl on PATH)
#   CLUSTER_NAME   cluster whose node containers are measured (default: snowops)
#   SETTLE_SECONDS time to let a scenario settle before sampling (default: 120)
#
# Memory is read in two ways: `kubectl top pods`, grouped by Helm release and
# by namespace, attributes it to shared items; `docker stats` on the node
# containers gives the total. Whatever the total rises by beyond the shared
# items is the scenario's own requirement.

LABCTL="${LABCTL:-labctl}"
CLUSTER_NAME="${CLUSTER_NAME:-snowops}"
SETTLE_SECONDS="${SETTLE_SECONDS:-120}"

# node_mib prints the MiB the cluster's node containers use now.
node_mib() {
  local ids
  ids="$(docker ps -q --filter "label=k3d.cluster=${CLUSTER_NAME}")"
  [ -n "$ids" ] || {
    echo 0
    return
  }
  # shellcheck disable=SC2086 # one container id per word
  docker stats --no-stream --format '{{.MemUsage}}' $ids | awk '
    { split($1, a, /[A-Za-z]+/); v = a[1]; u = $1; sub(/^[0-9.]+/, "", u)
      if (u == "GiB") v *= 1024; else if (u == "KiB") v /= 1024; else if (u == "B") v /= 1048576
      total += v }
    END { printf "%d\n", total }'
}

# pod_mib prints "namespace pod MiB" for every pod, from metrics-server.
pod_mib() {
  kubectl top pods -A --no-headers 2>/dev/null | awk '
    { v = $4; u = v; sub(/[0-9.]+/, "", u); sub(/[A-Za-z]+$/, "", v)
      if (u == "Gi") v *= 1024; else if (u == "Ki") v /= 1024
      printf "%s %s %d\n", $1, $2, v }'
}

# release_mib <namespace> <release> sums the memory of a Helm release's pods.
release_mib() {
  local pods
  pods="$(kubectl get pods -n "$1" -l "app.kubernetes.io/instance=$2" -o name 2>/dev/null | sed 's|^pod/||')"
  [ -n "$pods" ] || {
    echo 0
    return
  }
  pod_mib | awk -v ns="$1" -v pods="$pods" '
    BEGIN { n = split(pods, p, "\n"); for (i = 1; i <= n; i++) want[p[i]] = 1 }
    $1 == ns && ($2 in want) { total += $3 }
    END { printf "%d\n", total }'
}

# namespace_mib <namespace> sums the memory of every pod in a namespace.
namespace_mib() {
  pod_mib | awk -v ns="$1" '$1 == ns { total += $3 } END { printf "%d\n", total }'
}

# helm_releases <scenario> prints "namespace release" for its helm components.
helm_releases() {
  awk '
    /^ *- name:/ { name = $3; gsub(/"/, "", name); ns = ""; helm = 0 }
    /^ *type: helm/ { helm = 1 }
    /^ *namespace:/ { ns = $2; gsub(/"/, "", ns) }
    helm && name != "" && /^ *(chart|version):/ { if (!(name in seen)) { seen[name] = 1; print (ns == "" ? "default" : ns), name } }
  ' "scenarios/$1/scenario.yaml" | sed 's/{{\.MonitoringNamespace}}/monitoring/'
}

measure() {
  local scenario="$1" before after ns_before ns_after release ns mib shared=0
  echo "# --- ${scenario}" >&2
  ns_before="$(kubectl get ns -o name | sort)"
  before="$(node_mib)"

  if ! "$LABCTL" scenario up "$scenario" --deploy-prereqs >/dev/null 2>&1; then
    echo "#   ${scenario}: activation failed; skipped" >&2
    "$LABCTL" scenario down "$scenario" >/dev/null 2>&1 || true
    return
  fi
  sleep "$SETTLE_SECONDS"
  after="$(node_mib)"
  ns_after="$(kubectl get ns -o name | sort)"

  while read -r ns release; do
    [ -n "$release" ] || continue
    mib="$(release_mib "$ns" "$release")"
    shared=$((shared + mib))
    echo "  release ${release}: ${mib}" >&2
  done < <(helm_releases "$scenario")

  for ns in $(comm -13 <(echo "$ns_before") <(echo "$ns_after") | sed 's|^namespace/||'); do
    mib="$(namespace_mib "$ns")"
    shared=$((shared + mib))
    echo "  new namespace ${ns}: ${mib}" >&2
  done

  local own=$((after - before - shared))
  [ "$own" -lt 0 ] && own=0
  echo "  total rise: $((after - before)) MiB; own: ${own} MiB" >&2
  printf '%s: %d\n' "$scenario" "$own"

  "$LABCTL" scenario down "$scenario" >/dev/null 2>&1 || true
}

echo "baseline (node containers now): $(node_mib) MiB" >&2
if [ "$#" -eq 0 ]; then
  for dir in scenarios/*/; do
    set -- "$@" "$(basename "$dir")"
  done
fi
echo "# own memory per scenario (MiB), for each scenario's requirements.memory"
for scenario in "$@"; do
  [ -f "scenarios/${scenario}/scenario.yaml" ] || continue
  measure "$scenario"
done
