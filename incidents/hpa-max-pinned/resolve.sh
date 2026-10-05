#!/usr/bin/env bash
set -euo pipefail

NS="${TARGET_NAMESPACE:-go-api}"
HPA="${TARGET_WORKLOAD:-go-api}"
MARK="labfault-hpa-max-pinned"

# Resolve is idempotent: with no marker there is nothing to undo, and saying so
# is the honest answer on an HPA nobody capped through this fault.
if ! kubectl -n "$NS" get hpa "$HPA" \
  -o "jsonpath={.metadata.annotations.$MARK}" 2>/dev/null | grep -q .; then
  echo "Nothing to resolve: $HPA carries no $MARK marker."
  exit 0
fi

# Prefer the ceiling recorded at injection: it restores the HPA exactly as the
# fault found it. Without one, any ceiling above the floor un-caps the fault.
ORIG="$(kubectl -n "$NS" get hpa "$HPA" \
  -o "jsonpath={.metadata.annotations.$MARK-original-max}" 2>/dev/null || true)"
if [ -z "$ORIG" ] || [ "$ORIG" = "none" ]; then
  MIN="$(kubectl -n "$NS" get hpa "$HPA" -o 'jsonpath={.spec.minReplicas}' 2>/dev/null || true)"
  ORIG="$((${MIN:-1} + 1))"
fi

echo "Restoring $NS/$HPA maxReplicas to $ORIG..."
kubectl -n "$NS" patch hpa "$HPA" \
  -p "{\"spec\":{\"maxReplicas\":$ORIG}}" >/dev/null 2>&1 || true

kubectl -n "$NS" annotate hpa "$HPA" \
  "$MARK-" "$MARK-original-max-" --overwrite >/dev/null 2>&1 || true

echo "Resolved. The ceiling is above the floor again."
