#!/usr/bin/env bash
# Passes (exit 0) when the fault is resolved: an HPA exists for the workload
# and it actually targets the workload's Deployment.
#
# Both halves are needed. Existence alone is what this check used to be — but
# an HPA named right and aimed at the wrong Deployment scales something else
# while this workload sits still, and the check would pass on a lab that is
# still broken. Grade the target, not just the name.
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"

REF="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.scaleTargetRef.name}' 2>/dev/null || true)"
if [ -z "$REF" ]; then
  echo "FAIL: no HPA $NS/$HPA — nothing turns metrics into replicas for the workload." >&2
  echo "  Stuck? 'labctl incident hint' walks you in." >&2
  exit 1
fi
if [ "$REF" != "$HPA" ]; then
  echo "FAIL: HPA $NS/$HPA targets Deployment \"$REF\", not the workload — it scales something else." >&2
  exit 1
fi

echo "OK: HPA $NS/$HPA exists and targets the workload."
