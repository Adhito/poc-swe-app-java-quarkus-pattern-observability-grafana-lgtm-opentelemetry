#!/usr/bin/env bash
#
# Recovery script for a recurring incident on this cluster: when a worker
# node is rebuilt/rejoins via a fresh `kubeadm join` (not just rebooted), it
# loses two imperative, one-off configs that do NOT survive a rejoin:
#   1. The Dev VM's SSH key (never re-copied to the "new" node)
#   2. CRI-O's registry trust drop-in (never re-applied by Ansible)
# Kubernetes-native things (Calico CNI, ArgoCD-managed workloads) self-heal
# automatically on rejoin; these two do not, because they're not part of
# cluster provisioning. Symptom: new pods scheduled on that node sit in
# ImagePullBackOff with "http: server gave HTTP response to HTTPS client".
# See documents/CLAUDE-CODE-FIXING-LOG.md (2026-07-29 entry) for the full
# story, and documents/runbook.md ("Standing checklist: after any node
# incident") for the manual version of these same steps.
#
# Usage:
#   ./scripts/utility-node-registry-recovery.sh <node-ip> [namespace]
#
# Example:
#   ./scripts/utility-node-registry-recovery.sh 192.168.56.12 tracing-poc
#
# Run from the Dev VM (from anywhere inside the repo — repo root is
# auto-detected). You will be prompted once for the node's SSH password
# (the Vagrant box default) unless the key is already trusted.

set -euo pipefail

NODE_IP="${1:?Usage: $0 <node-ip> [namespace]}"
NAMESPACE="${2:-tracing-poc}"
SSH_USER="${SSH_USER:-vagrant}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ANSIBLE_DIR="$REPO_ROOT/src-infra/ansible"

echo "==> Step 1/4: copying SSH key to $SSH_USER@$NODE_IP"
echo "    (password prompt only appears if the key isn't already trusted)"
ssh-copy-id "$SSH_USER@$NODE_IP"

echo "==> Step 2/4: re-running registry-trust playbook (idempotent — safe against all nodes)"
(
  cd "$ANSIBLE_DIR"
  if [ ! -f inventory.ini ]; then
    echo "    inventory.ini missing, copying from inventory.example.ini"
    cp inventory.example.ini inventory.ini
  fi
  ansible-playbook -i inventory.ini registry-trust.yaml
)

echo "==> Step 3/4: force-deleting any ImagePullBackOff/ErrImagePull pods in namespace '$NAMESPACE'"
mapfile -t STUCK_PODS < <(
  kubectl get pods -n "$NAMESPACE" --no-headers 2>/dev/null \
    | awk '$3 ~ /ImagePullBackOff|ErrImagePull/ {print $1}'
)

if [ "${#STUCK_PODS[@]}" -eq 0 ]; then
  echo "    no stuck pods found in namespace $NAMESPACE"
else
  echo "    deleting: ${STUCK_PODS[*]}"
  kubectl delete pod -n "$NAMESPACE" "${STUCK_PODS[@]}"
fi

echo "==> Step 4/4: waiting 15s for pods to settle, then showing status"
sleep 15
kubectl get pods -n "$NAMESPACE" -o wide

echo
echo "Done. If anything is still ImagePullBackOff, wait another minute and re-run:"
echo "  kubectl get pods -n $NAMESPACE"
