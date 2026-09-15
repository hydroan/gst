#!/usr/bin/env bash
# Takes the example down: every object the manifests created, the MySQL and
# its data included.
set -euo pipefail
cd "$(dirname "$0")/.."

kubectl delete -f deploy/k8s/ --ignore-not-found
