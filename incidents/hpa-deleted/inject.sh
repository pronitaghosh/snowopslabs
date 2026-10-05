#!/usr/bin/env bash
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"
DEPLOY="${TARGET_WORKLOAD:-go-api}"
MARK="labfault-hpa-deleted"

# This fault removes an existing autoscaler, so it needs one. Applying the
# workload's HPA is the drill setup (autoscaling-basics), not part of the fault.
if ! kubectl -n "$NS" get hpa "$HPA" >/dev/null 2>&1; then
  if kubectl -n "$NS" get deploy "$DEPLOY" \
    -o "jsonpath={.metadata.annotations.$MARK}" 2>/dev/null | grep -q .; then
    echo "Fault already injected — HPA $NS/$HPA is already gone."
    exit 0
  fi
  echo "No HPA $NS/$HPA to delete — apply the workload's autoscaler first:" >&2
  echo "  labctl scenario render autoscaling-basics manifests/hpa.yaml --app $HPA | kubectl apply -f -" >&2
  exit 1
fi

# The object is about to disappear, so the marker and the original spec live on
# the Deployment instead. The spec is compact JSON without whitespace, which an
# annotation value carries without quoting trouble.
SPEC="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec}' 2>/dev/null || true)"
if [ -z "$SPEC" ] || [ "$SPEC" = "{}" ]; then
  echo "FAIL: could not read HPA $NS/$HPA spec — refusing to delete what cannot be restored." >&2
  exit 1
fi

echo "Deleting HPA $NS/$HPA (spec stashed on deployment $NS/$DEPLOY)..."
kubectl -n "$NS" annotate deploy "$DEPLOY" \
  "$MARK=injected" "$MARK-original-hpa-spec=$SPEC" --overwrite >/dev/null
kubectl -n "$NS" delete hpa "$HPA" >/dev/null

echo "Done."
