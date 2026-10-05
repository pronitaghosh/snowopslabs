#!/usr/bin/env bash
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"
MARK="labfault-hpa-max-pinned"

# This fault caps an existing autoscaler, so it needs one. Applying the
# workload's HPA is the drill setup (autoscaling-basics), not part of the fault.
if ! kubectl -n "$NS" get hpa "$HPA" >/dev/null 2>&1; then
  echo "No HPA $NS/$HPA to cap — apply the workload's autoscaler first:" >&2
  echo "  labctl scenario render autoscaling-basics manifests/hpa.yaml --app $HPA | kubectl apply -f -" >&2
  exit 1
fi

MIN="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.minReplicas}' 2>/dev/null || true)"
MAX="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.maxReplicas}' 2>/dev/null || true)"
MIN="${MIN:-1}"

# Guard on the fault (ceiling at the floor), not the marker: a marker can
# outlive a resolve, and then a healthy HPA refuses injection on a lab that
# simply runs capped by its owner's hand.
if [ -n "$MAX" ] && [ "$MAX" -le "$MIN" ]; then
  echo "Fault already injected — maxReplicas is already at the floor ($MIN)."
  exit 0
fi

echo "Pinning $NS/$HPA maxReplicas to the floor ($MIN)..."
kubectl -n "$NS" annotate hpa "$HPA" \
  "$MARK=injected" "$MARK-original-max=${MAX:-none}" --overwrite >/dev/null
kubectl -n "$NS" patch hpa "$HPA" \
  -p "{\"spec\":{\"maxReplicas\":$MIN}}" >/dev/null

echo "Done."
