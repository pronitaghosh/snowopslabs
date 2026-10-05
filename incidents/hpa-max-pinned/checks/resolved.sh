#!/usr/bin/env bash
# Passes (exit 0) when the fault is resolved: the HPA exists and its ceiling
# sits above its floor, so load can actually add replicas.
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"

MIN="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.minReplicas}' 2>/dev/null || true)"
MAX="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.maxReplicas}' 2>/dev/null || true)"
if [ -z "$MAX" ]; then
  echo "FAIL: HPA $NS/$HPA is missing — nothing can scale the workload." >&2
  exit 1
fi
MIN="${MIN:-1}"
if [ "$MAX" -le "$MIN" ]; then
  echo "FAIL: HPA $NS/$HPA pins maxReplicas ($MAX) at the floor ($MIN) — replicas cannot move under load." >&2
  echo "  Stuck? 'labctl incident hint' walks you in." >&2
  exit 1
fi

echo "OK: HPA $NS/$HPA allows $MIN->$MAX replicas."
