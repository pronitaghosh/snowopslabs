#!/usr/bin/env bash
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"
DEPLOY="${TARGET_WORKLOAD:-go-api}"
MARK="labfault-hpa-deleted"

# Resolve is idempotent: with no marker there is nothing to undo, and saying so
# is the honest answer on a lab nobody broke through this fault.
if ! kubectl -n "$NS" get deploy "$DEPLOY" \
  -o "jsonpath={.metadata.annotations.$MARK}" 2>/dev/null | grep -q .; then
  echo "Nothing to resolve: $DEPLOY carries no $MARK marker."
  exit 0
fi

# A hand-reapplied HPA is a genuine fix by another route — keep it and drop the
# bookkeeping rather than overwriting the learner's work.
if kubectl -n "$NS" get hpa "$HPA" >/dev/null 2>&1; then
  echo "HPA $NS/$HPA already exists again — keeping it and clearing the marker."
  kubectl -n "$NS" annotate deploy "$DEPLOY" \
    "$MARK-" "$MARK-original-hpa-spec-" --overwrite >/dev/null 2>&1 || true
  exit 0
fi

SPEC="$(kubectl -n "$NS" get deploy "$DEPLOY" \
  -o "jsonpath={.metadata.annotations.$MARK-original-hpa-spec}" 2>/dev/null || true)"
if [ -z "$SPEC" ] || [ "$SPEC" = "{}" ] || [ "$SPEC" = "none" ]; then
  echo "FAIL: cannot determine $HPA's original spec — recreate it by hand:" >&2
  echo "  labctl scenario render autoscaling-basics manifests/hpa.yaml --app $HPA --namespace $NS | kubectl apply -f -" >&2
  exit 1
fi

echo "Recreating HPA $NS/$HPA from the stashed spec..."
kubectl -n "$NS" apply -f - >/dev/null <<MANIFEST
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: $HPA
  namespace: $NS
spec: $SPEC
MANIFEST

kubectl -n "$NS" annotate deploy "$DEPLOY" \
  "$MARK-" "$MARK-original-hpa-spec-" --overwrite >/dev/null 2>&1 || true

echo "Resolved. The autoscaler is back."
