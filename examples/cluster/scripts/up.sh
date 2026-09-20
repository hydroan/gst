#!/usr/bin/env bash
# Brings the example up on the current kubectl context: builds the image,
# applies the manifests under deploy/k8s — the gst-cluster namespace, one
# MySQL, one Kafka broker and three replicas — and waits until every pod is
# ready. The image is built from the repository root, since the example's
# go.mod points at the framework sources beside it. A cluster that cannot see the local Docker
# images (kind, minikube) needs the image loaded first; see the README.
#
# Run again after a change, it rolls the replicas over to the new build: the
# tag and the manifests stay the same, so applying them alone would leave the
# running pods on the old image.
set -euo pipefail
cd "$(dirname "$0")/../../.."

docker build -f examples/cluster/Dockerfile -t gst-cluster:dev .
existing=$(kubectl -n gst-cluster get deployment cluster --ignore-not-found -o name)
kubectl apply -f examples/cluster/deploy/k8s/
if [ -n "$existing" ]; then
  kubectl -n gst-cluster rollout restart deployment/cluster
fi
kubectl -n gst-cluster rollout status statefulset/mysql --timeout=300s
kubectl -n gst-cluster rollout status statefulset/kafka --timeout=300s
kubectl -n gst-cluster rollout status deployment/cluster --timeout=300s
kubectl -n gst-cluster get pods
