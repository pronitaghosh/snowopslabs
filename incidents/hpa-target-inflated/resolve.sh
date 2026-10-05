#!/usr/bin/env bash
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"
MARK="labfault-hpa-target-inflated"

# Resolve is idempotent: with no marker there is nothing to undo, and saying so
# is the honest answer on an HPA nobody inflated through this fault.
if ! kubectl -n "$NS" get hpa "$HPA" \
  -o "jsonpath={.metadata.annotations.$MARK}" 2>/dev/null | grep -q .; then
  echo "Nothing to resolve: $HPA carries no $MARK marker."
  exit 0
fi

# Prefer the target recorded at injection: it restores the HPA exactly as the
# fault found it. Without one, any sane target un-inflates the fault.
ORIG="$(kubectl -n "$NS" get hpa "$HPA" \
  -o "jsonpath={.metadata.annotations.$MARK-original-target}" 2>/dev/null || true)"
if [ -z "$ORIG" ] || [ "$ORIG" = "none" ]; then
  ORIG=60
fi

# Only the single-metric CPU shape this fault fits can be restored by writing
# the whole metric back. Anything else changed shape by hand — say how to fix
# it rather than clobbering triggers resolve cannot see.
COUNT="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.metrics[*].type}' 2>/dev/null | wc -w | tr -d ' ' || true)"
if [ "$COUNT" != "1" ]; then
  echo "FAIL: HPA $NS/$HPA no longer has one metric — restore its CPU target by hand:" >&2
  echo "  kubectl -n $NS patch hpa $HPA -p '{\"spec\":{\"metrics\":[{\"type\":\"Resource\",\"resource\":{\"name\":\"cpu\",\"target\":{\"type\":\"Utilization\",\"averageUtilization\":$ORIG}}}]}}'" >&2
  exit 1
fi

echo "Restoring $NS/$HPA CPU target to ${ORIG}%..."
kubectl -n "$NS" patch hpa "$HPA" \
  -p "{\"spec\":{\"metrics\":[{\"type\":\"Resource\",\"resource\":{\"name\":\"cpu\",\"target\":{\"type\":\"Utilization\",\"averageUtilization\":$ORIG}}}]}}" >/dev/null 2>&1 || true

kubectl -n "$NS" annotate hpa "$HPA" \
  "$MARK-" "$MARK-original-target-" --overwrite >/dev/null 2>&1 || true

echo "Resolved. The target reacts before saturation again."
