#!/usr/bin/env bash
set -euo pipefail

# Create a k3d cluster
# Expose HTTP/HTTPS ports on the host so that ingress rules using
# hostnames resolve to localhost and traffic reaches the load-balancer pod.
# Values come from .env (via labctl config) or environment variables.

CLUSTER_NAME="${1:-${CLUSTER_NAME:-snowops}}"
HTTP_PORT="${HTTP_PORT:-80}"
HTTPS_PORT="${HTTPS_PORT:-443}"
# Number of agent (worker) nodes. Multi-node by default so day-2 drills
# (node drain, rolling upgrade) have somewhere to reschedule pods.
AGENTS="${AGENTS:-2}"
# Optional k3s version pin (e.g. K3S_VERSION=v1.28.8-k3s1). Empty = k3d default.
# The cluster-upgrade-drill creates a cluster pinned to an older version, then
# rolls the agents to a newer one.
K3S_VERSION="${K3S_VERSION:-}"
# How long to wait for an existing cluster's API after a restart, per attempt.
REACHABLE_WAIT="${REACHABLE_WAIT:-180}"
# How long to keep waiting (and restarting stuck nodes) for every node to be Ready.
NODE_READY_WAIT="${NODE_READY_WAIT:-300}"
# Consecutive healthy samples, NODE_CHECK_INTERVAL seconds apart, before a
# restarted cluster counts as settled.
NODE_STABLE_CHECKS="${NODE_STABLE_CHECKS:-6}"
NODE_CHECK_INTERVAL="${NODE_CHECK_INTERVAL:-10}"

# shellcheck source=../_lib/docker.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/_lib/docker.sh"

ensure_docker_running || exit 1

# ---------------------------------------------------------------------------
# Normalise the kubeconfig API-server host.
#
# k3d writes the server as https://0.0.0.0:<port>. 0.0.0.0 is a bind-all
# address, not a stable destination: on macOS (Docker Desktop / Colima) a
# client connecting to it intermittently stalls the TLS handshake, which
# surfaces as "TLS handshake timeout" / "http2: client connection lost" partway
# through a helm install. Rewriting the host to 127.0.0.1 is reliable on macOS
# and Linux alike (golden rule 1).
# ---------------------------------------------------------------------------
normalize_apiserver_host() {
  local ctx="k3d-$CLUSTER_NAME" server fixed
  server="$(kubectl config view -o jsonpath="{.clusters[?(@.name=='${ctx}')].cluster.server}" 2>/dev/null || true)"
  [ -n "$server" ] || return 0
  fixed="${server//0.0.0.0/127.0.0.1}"
  if [ "$server" != "$fixed" ]; then
    echo "Rewriting API server ${server} -> ${fixed} for reliable access on macOS."
    kubectl config set-cluster "$ctx" --server="$fixed" >/dev/null
  fi
}

# Return 0 only if the cluster's API server actually answers. This is what tells
# a healthy existing cluster (skip creation) apart from a half-built one whose
# containers linger but whose API never comes up — e.g. a load-balancer that
# failed to initialise, or a kubeconfig entry that was removed out from under it.
cluster_reachable() {
  kubectl config use-context "k3d-$CLUSTER_NAME" >/dev/null 2>&1 || return 1
  kubectl --request-timeout=20s get --raw=/healthz >/dev/null 2>&1
}

# port_free <port> — 0 if nothing is listening on the host TCP port, non-zero if
# it is already taken. Uses bash's /dev/tcp so it needs no nc/lsof/ss (which
# differ across macOS and Linux — golden rule 1). The subshell scopes fd 3.
port_free() {
  ! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

# pick_port <preferred> <fallback-base> — echo <preferred> if it is free, else
# the first free port at or above <fallback-base>. Lets a second cluster come up
# on one host: k3d's load-balancer cannot bind a host port another cluster (or
# any service) already holds, and would otherwise sit unstarted forever.
pick_port() {
  local preferred=$1 base=$2 p
  if port_free "$preferred"; then
    printf '%s' "$preferred"
    return 0
  fi
  p=$base
  while ! port_free "$p"; do
    p=$((p + 1))
    if [ "$p" -gt $((base + 100)) ]; then
      echo "ERROR: no free host port found near ${base} for ingress." >&2
      return 1
    fi
  done
  printf '%s' "$p"
}

# Raise fs.inotify.max_user_instances on every node. The default (128) is quickly
# exhausted by log/file watchers — most visibly promtail (observability-sre),
# which crash-loops with "too many open files: failed to make file target
# manager". k3d nodes run privileged and share the colima/Docker VM kernel, so
# setting the sysctl on each node container raises the effective limit for the
# whole cluster. Best-effort: never fail cluster bringup over this.
raise_inotify_limits() {
  local node
  # k3d nodes share the kernel of whatever runs Docker. Under colima, Docker
  # Desktop or WSL that is a VM; on native Linux it is this machine.
  if [ "$(uname -s)" = "Linux" ] && [ -z "${WSL_DISTRO_NAME:-}" ]; then
    echo "Raising fs.inotify.max_user_instances to 512 (on native Linux this is your host's kernel setting)."
  fi
  for node in $(docker ps --filter "label=k3d.cluster=${CLUSTER_NAME}" --format '{{.Names}}' 2>/dev/null); do
    docker exec "$node" sysctl -w fs.inotify.max_user_instances=512 >/dev/null 2>&1 || true
  done
}

# A node container can be "Up" with no k3s running inside it. When Docker (or
# the Colima VM under it) restarts, the node containers come back with
# reshuffled bridge IPs and k3s exits immediately -- the address recorded for
# the node no longer matches any local interface ("failed to find interface
# with specified node ip"). k3d's entrypoint then loops on `kubectl uncordon`
# forever, so the container stays Up, Docker never restarts it, and the node
# sits NotReady indefinitely. dead_nodes prints the server and agent
# containers with no k3s process in them.
dead_nodes() {
  local node role
  for node in $(docker ps --filter "label=k3d.cluster=${CLUSTER_NAME}" --format '{{.Names}}' 2>/dev/null); do
    role="$(docker inspect "$node" --format '{{index .Config.Labels "k3d.role"}}' 2>/dev/null || true)"
    # Only server and agent containers run k3s; the load-balancer runs nginx and
    # exits on its own when it dies, so Docker restarts it without our help.
    case "$role" in
      server | agent) ;;
      *) continue ;;
    esac
    # Matched on a captured string, not through a pipe: `set -o pipefail` plus
    # grep -q's early exit reports the whole pipeline as failed on a match.
    case "$(docker top "$node" 2>/dev/null || true)" in
      */bin/k3s*) ;;
      *) printf '%s\n' "$node" ;;
    esac
  done
}


# wait_until_reachable <seconds> — succeed once the API answers. After Docker
# or the colima VM restarts, k3s takes a minute or more to serve again; judging
# it sooner once deleted a perfectly good lab.
wait_until_reachable() {
  local limit="$1" waited=0
  until cluster_reachable; do
    if [ "$waited" -ge "$limit" ]; then
      return 1
    fi
    sleep 5
    waited=$((waited + 5))
  done
}


# reregister_if_moved <node> — when a node's container has a different IP
# from the one its Node object records, delete the Node and restart the
# container so it registers again at its current address. k3s's network policy
# controller reads the recorded address before the kubelet can update it, so
# such a node shuts down on every restart ("failed to find interface with
# specified node ip"). The node keeps its name, so PersistentVolumes pinned to
# it stay valid; its pods are recreated. Succeeds only when it acted.
reregister_if_moved() {
  local node="$1" have recorded
  have="$(docker inspect "$node" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null || true)"
  recorded="$(kubectl get node "$node" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || true)"
  if [ -z "$have" ] || [ -z "$recorded" ] || [ "$have" = "$recorded" ]; then
    return 1
  fi
  echo "Node '$node' is registered at ${recorded} but its container now has ${have} — registering it again."
  kubectl delete node "$node" --wait=false >/dev/null 2>&1 || true
  docker restart "$node" >/dev/null 2>&1 || true
}

# ensure_nodes_ready waits until every node runs k3s and is Ready, and stays
# that way for NODE_STABLE_CHECKS samples 10 s apart. After a VM restart an
# agent's k3s can die a minute after starting ("failed to find interface with
# specified node ip"), while its Node still reads Ready from before, so one
# early look is not enough. A node found dead or NotReady is restarted — at
# most twice — which makes k3s re-read the address its container has now.
ensure_nodes_ready() {
  local deadline=$((SECONDS + NODE_READY_WAIT)) stable=0 node bad restarts=""
  echo "Waiting for every node to be Ready and stay up..."
  while :; do
    bad="$(dead_nodes)"
    if [ -z "$bad" ] && kubectl wait --for=condition=Ready node --all --timeout=20s >/dev/null 2>&1; then
      stable=$((stable + 1))
      [ "$stable" -ge "$NODE_STABLE_CHECKS" ] && return 0
    else
      stable=0
      bad="$bad $(kubectl get nodes --no-headers 2>/dev/null | awk '$2 !~ /^Ready/ {print $1}')"
      for node in $bad; do
        # restarts lists a node once per restart; allow two per node.
        [ "$(printf '%s\n' $restarts | grep -cx "$node" || true)" -ge 2 ] && continue
        restarts="$restarts$node "
        reregister_if_moved "$node" && continue
        echo "Node '$node' is not healthy — restarting it."
        docker restart "$node" >/dev/null 2>&1 || true
      done
    fi
    [ "$SECONDS" -ge "$deadline" ] && return 1
    sleep "$NODE_CHECK_INTERVAL"
  done
}

# k3d_images prints the images `k3d cluster create` will need: the k3s node
# image (pinned or k3d's default) and k3d's own helper images.
k3d_images() {
  local out k3d_ver k3s_ver
  out="$(k3d version 2>/dev/null || true)"
  k3d_ver="$(printf '%s\n' "$out" | sed -n 's/^k3d version \(v[^ ]*\).*/\1/p' | head -n 1)"
  k3s_ver="${K3S_VERSION:-$(printf '%s\n' "$out" | sed -n 's/^k3s version \(v[^ ]*\).*/\1/p' | head -n 1)}"
  [ -n "$k3s_ver" ] && printf 'rancher/k3s:%s\n' "$k3s_ver"
  if [ -n "$k3d_ver" ]; then
    printf 'ghcr.io/k3d-io/k3d-tools:%s\n' "${k3d_ver#v}"
    printf 'ghcr.io/k3d-io/k3d-proxy:%s\n' "${k3d_ver#v}"
  fi
  return 0
}

# Disable the bundled Traefik so we manage our own install in the traefik namespace.
# This prevents two competing Traefik instances from causing 404 errors.
create_cluster() {
  # Fall back to free host ports when the defaults are already bound (e.g. another
  # k3d cluster is running). Keeps the golden path on 80/443 untouched.
  local http_port https_port
  http_port="$(pick_port "$HTTP_PORT" 8080)" || exit 1
  https_port="$(pick_port "$HTTPS_PORT" 8443)" || exit 1
  if [ "$http_port" != "$HTTP_PORT" ] || [ "$https_port" != "$HTTPS_PORT" ]; then
    echo "Host ports ${HTTP_PORT}/${HTTPS_PORT} are already in use (another cluster or service)."
    echo "Exposing ingress on ${http_port}/${https_port} instead — reach services at" \
      "http://<name>.${DOMAIN_SUFFIX:-k3d.local}:${http_port}"
    HTTP_PORT="$http_port"
    HTTPS_PORT="$https_port"
  fi

  local create_args=(
    "$CLUSTER_NAME"
    --agents "$AGENTS"
    -p "${HTTP_PORT}:80@loadbalancer"
    -p "${HTTPS_PORT}:443@loadbalancer"
    --k3s-arg "--disable=traefik@server:*"
  )
  if [ -n "$K3S_VERSION" ]; then
    echo "Pinning k3s version to ${K3S_VERSION}"
    create_args+=(--image "rancher/k3s:${K3S_VERSION}")
  fi

  # Pull the images first, with progress and a stall timeout: k3d pulls them
  # silently, so a stalled download used to look like a hung `init`.
  echo "Downloading cluster images (first run only, a few hundred MB)..."
  local images=() img
  while IFS= read -r img; do
    [ -n "$img" ] && images+=("$img")
  done < <(k3d_images)
  # The +-expansion keeps bash 3.2 (macOS) quiet about an empty array under set -u.
  prepull_images ${images[@]+"${images[@]}"} || exit 1

  # --timeout rolls back a creation that never finishes; one retry covers a
  # transient failure without leaving a half-built cluster behind.
  if ! k3d cluster create "${create_args[@]}" --timeout 300s; then
    echo "Cluster creation failed — cleaning up and retrying once..." >&2
    k3d cluster delete "$CLUSTER_NAME" >/dev/null 2>&1 || true
    k3d cluster create "${create_args[@]}" --timeout 300s || {
      echo "ERROR: k3d could not create the cluster. 'labctl doctor' checks Docker's resources." >&2
      exit 1
    }
  fi

  kubectl config use-context "k3d-$CLUSTER_NAME"
  normalize_apiserver_host
}

# An existing cluster is only worth keeping if its API answers. k3d can leave a
# cluster half-built (containers present but the load-balancer or kubeconfig
# broken). The old code assumed "exists" meant "usable" and ran
# `kubectl config use-context` against a context that no longer existed, which
# aborts `make init` under `set -e`. Instead: refresh the kubeconfig entry, and
# skip creation only when the API is reachable. A broken cluster is recreated
# rather than left to fail every step that follows.
# lab_healthy is the quick look: API answering, k3s in every node container,
# and every node Ready.
lab_healthy() {
  cluster_reachable &&
    [ -z "$(dead_nodes)" ] &&
    kubectl wait --for=condition=Ready node --all --timeout=20s >/dev/null 2>&1
}

# restart_in_order restarts the cluster the way k3d does it: servers first,
# waited for, then agents. After Docker or the colima VM restarts, Docker brings
# every node back at once, and an agent that starts while its server is still
# starting can fail for good ("failed to find interface with specified node
# ip"); restarting nodes one by one in no order repeats the same race.
restart_in_order() {
  echo "Restarting the cluster in order (servers, then agents)..."
  k3d cluster stop "$CLUSTER_NAME" >/dev/null 2>&1 || true
  k3d cluster start "$CLUSTER_NAME" --wait --timeout 300s || true
  k3d kubeconfig merge "$CLUSTER_NAME" --kubeconfig-merge-default &>/dev/null || true
  normalize_apiserver_host
}

if k3d cluster list "$CLUSTER_NAME" &>/dev/null; then
  echo "Cluster '$CLUSTER_NAME' already exists — checking it is healthy."
  # Rebuild the kubeconfig entry in case it was removed or points at a stale port.
  k3d kubeconfig merge "$CLUSTER_NAME" --kubeconfig-merge-default &>/dev/null || true
  normalize_apiserver_host
  if ! lab_healthy; then
    restart_in_order
  fi
  if ! wait_until_reachable "$REACHABLE_WAIT"; then
    # Never delete an existing cluster on our own: it holds the user's apps,
    # scenarios and dashboards. Rebuilding is their call.
    echo "ERROR: cluster '$CLUSTER_NAME' exists but its API server is not answering." >&2
    echo "  Check Docker's memory with 'labctl doctor' and the nodes with 'docker ps'." >&2
    echo "  To rebuild the lab from scratch (this loses its apps and scenarios): labctl reset" >&2
    exit 1
  fi
  if ! ensure_nodes_ready; then
    echo "ERROR: some nodes of '$CLUSTER_NAME' stay NotReady: $(kubectl get nodes --no-headers 2>/dev/null | awk '$2 !~ /^Ready/ {printf "%s ", $1}')" >&2
    echo "  Check Docker's memory with 'labctl doctor'. To rebuild the lab from scratch (loses its state): labctl reset" >&2
    exit 1
  fi
  echo "Cluster '$CLUSTER_NAME' is healthy; skipping creation."
  raise_inotify_limits
  record_ingress_ports "$CLUSTER_NAME" "k3d-${CLUSTER_NAME}-serverlb"
  exit 0
fi

create_cluster
raise_inotify_limits
record_ingress_ports "$CLUSTER_NAME" "k3d-${CLUSTER_NAME}-serverlb"
