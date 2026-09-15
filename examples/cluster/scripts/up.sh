#!/usr/bin/env bash
# Brings the example up on the current kubectl context: builds the image,
# applies the manifests under deploy/k8s — one MySQL and three replicas —
# and waits until every pod is ready. The image is built from the
# repository root, since the example's go.mod points at the framework
# sources beside it. A cluster that cannot see the local Docker images
# (kind, minikube) needs the image loaded first; see the README.
set -euo pipefail
cd "$(dirname "$0")/../../.."

docker build -f examples/cluster/Dockerfile -t gst-cluster:dev .
kubectl apply -f examples/cluster/deploy/k8s/
kubectl rollout status deployment/mysql --timeout=180s
kubectl rollout status deployment/cluster --timeout=300s
kubectl get pods -l app=cluster
