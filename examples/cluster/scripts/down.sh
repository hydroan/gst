#!/usr/bin/env bash
# Takes the example down: the gst-cluster namespace and everything in it, the
# MySQL and its data included.
set -euo pipefail

kubectl delete namespace gst-cluster --ignore-not-found
