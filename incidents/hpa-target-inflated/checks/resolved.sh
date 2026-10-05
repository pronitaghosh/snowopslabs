#!/usr/bin/env bash
# Passes (exit 0) when the fault is resolved: the HPA carries a CPU target that
# reacts before saturation, so scaling starts while headroom remains.
#
# The bound is deliberately generous (at or under 80): the fault pushes the
# target to 95, and anything in the sane range is a genuine fix — including a
# value the owner chose that differs from the drill default.
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"

TGT="$(kubectl -n "$NS" get hpa "$HPA" \
  -o 'jsonpath={.spec.metrics[0].resource.target.averageUtilization}' 2>/dev/null || true)"
if [ -z "$TGT" ]; then
  echo "FAIL: HPA $NS/$HPA has no CPU utilization target to scale on." >&2
  echo "  Stuck? 'labctl incident hint' walks you in." >&2
  exit 1
fi
if [ "$TGT" -gt 80 ]; then
  echo "FAIL: HPA $NS/$HPA targets ${TGT}% CPU — it holds back until the service is nearly saturated." >&2
  echo "  Stuck? 'labctl incident hint' walks you in." >&2
  exit 1
fi

echo "OK: HPA $NS/$HPA targets ${TGT}% CPU."
