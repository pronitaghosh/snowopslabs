#!/usr/bin/env bash
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"
MARK="labfault-hpa-target-inflated"
BAD_TARGET=95

# This fault retunes an existing CPU autoscaler, so it needs a single-metric
# CPU HPA — the shape the autoscaling-basics drill teaches. A multi-trigger
# autoscaler (KEDA) has no one target to inflate, and patching its metric list
# would drop triggers rather than retune them.
if ! kubectl -n "$NS" get hpa "$HPA" >/dev/null 2>&1; then
  echo "No HPA $NS/$HPA to retune — apply the workload's autoscaler first:" >&2
  echo "  labctl scenario render autoscaling-basics manifests/hpa.yaml --app $HPA | kubectl apply -f -" >&2
  exit 1
fi
COUNT="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.metrics[*].type}' 2>/dev/null | wc -w | tr -d ' ' || true)"
KIND="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.metrics[0].resource.name}' 2>/dev/null || true)"
if [ "$COUNT" != "1" ] || [ "$KIND" != "cpu" ]; then
  echo "HPA $NS/$HPA is not a single-metric CPU autoscaler — this fault does not fit it." >&2
  exit 1
fi

CUR="$(kubectl -n "$NS" get hpa "$HPA" \
  -o 'jsonpath={.spec.metrics[0].resource.target.averageUtilization}' 2>/dev/null || true)"
# Guard on the fault (target near saturation), not the marker: a marker can
# outlive a resolve, and then a lab someone tuned hot by hand refuses injection.
if [ -n "$CUR" ] && [ "$CUR" -ge 90 ]; then
  echo "Fault already injected — CPU target is already near saturation ($CUR%)."
  exit 0
fi

echo "Inflating $NS/$HPA CPU target ${CUR:-unknown}% -> ${BAD_TARGET}%..."
kubectl -n "$NS" annotate hpa "$HPA" \
  "$MARK=injected" "$MARK-original-target=${CUR:-none}" --overwrite >/dev/null
kubectl -n "$NS" patch hpa "$HPA" \
  -p "{\"spec\":{\"metrics\":[{\"type\":\"Resource\",\"resource\":{\"name\":\"cpu\",\"target\":{\"type\":\"Utilization\",\"averageUtilization\":$BAD_TARGET}}}]}}" >/dev/null

echo "Done."
