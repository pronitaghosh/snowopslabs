#!/usr/bin/env bash
set -euo pipefail
. "$(dirname "$0")/../../_lib/workload.sh"

# Deletes the learner-applied HPA. Runs on activation for a clean start after a
# previous run, and on teardown so no autoscaler orphans the scenario.
kubectl -n "$WORKLOAD_NAMESPACE" delete hpa "$WORKLOAD_NAME" --ignore-not-found
