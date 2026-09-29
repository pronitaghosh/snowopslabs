#!/usr/bin/env bash
set -euo pipefail

# Move a local lab's hostnames from one domain suffix to another, in place.
# Every Ingress, Traefik IngressRoute and cert-manager Certificate naming a host
# under <old> is rewritten to <new>, then the cluster record points every URL
# and check at <new>. Helm installs read the record, so upgrades keep <new>.
# Usage: move-domain.sh <cluster> <old-suffix> <new-suffix>

CLUSTER_NAME="$1"
OLD="$2"
NEW="$3"

# shellcheck source=docker.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/docker.sh"

# The old suffix as a sed pattern: its dots match only dots.
old_re="$(printf '%s' "$OLD" | sed 's/\./\\./g')"

# rewrite_hosts replaces ".<old>" with ".<new>" wherever it ends a hostname,
# so a longer name that merely starts with <old> is left alone.
rewrite_hosts() {
  sed -e "s/\.${old_re}\([^A-Za-z0-9.-]\)/.${NEW}\1/g" -e "s/\.${old_re}\$/.${NEW}/"
}

# move_kind rewrites every resource of <kind> that names a host under <old>.
# A kind the cluster does not serve (its CRD is not installed) is skipped.
move_kind() {
  local kind="$1" list ns name manifest
  list="$(kubectl get "$kind" -A \
    -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}' 2>/dev/null)" || return 0
  while read -r ns name; do
    [ -n "$name" ] || continue
    manifest="$(kubectl get "$kind" -n "$ns" "$name" -o yaml)"
    case "$manifest" in
      *".$OLD"*) ;;
      *) continue ;;
    esac
    printf '%s\n' "$manifest" | rewrite_hosts | kubectl replace -f - >/dev/null
    echo "  ${kind%%.*} ${ns}/${name}"
  done <<EOF
$list
EOF
}

echo "Moving the hostnames of '$CLUSTER_NAME' from *.${OLD} to *.${NEW}..."
move_kind ingresses
move_kind ingressroutes.traefik.io
move_kind certificates.cert-manager.io

http="$(record_value "$CLUSTER_NAME" HTTP_PORT)"
https="$(record_value "$CLUSTER_NAME" HTTPS_PORT)"
write_record "$CLUSTER_NAME" "${http:-80}" "${https:-443}" "$NEW"
echo "Lab URLs now end in .${NEW}."
