#!/usr/bin/env bash
set -euo pipefail

# Move a k3d cluster's ingress to free host ports, for when the ports it has are
# answered by something else on this machine (another lab, or another user's
# Docker VM). k3d adds the ports to the load balancer without touching the
# cluster, and the record then points every URL and check at them.
# Usage: move-ingress.sh <cluster>

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"

# shellcheck source=../_lib/docker.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/docker.sh"

http="$(pick_port 8080 8080)" || exit 1
https="$(pick_port 8443 8443)" || exit 1
suffix="$(record_value "$CLUSTER_NAME" DOMAIN_SUFFIX)"
[ -n "$suffix" ] || suffix="${DOMAIN_SUFFIX:-${CLUSTER_NAME}.localhost}"

echo "Adding host ports ${http}/${https} to the load balancer of '$CLUSTER_NAME'..."
k3d cluster edit "$CLUSTER_NAME" --port-add "${http}:80@loadbalancer" --port-add "${https}:443@loadbalancer"
write_record "$CLUSTER_NAME" "$http" "$https" "$suffix"
echo "The lab's ingress now listens on ${http}/${https}."
