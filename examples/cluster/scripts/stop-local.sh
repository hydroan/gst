#!/usr/bin/env bash
# Stops the replicas run-local.sh started, gracefully: SIGTERM starts the
# framework's shutdown, which releases the leases the replica holds.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -f .pids ] || { echo "nothing to stop"; exit 0; }
while read -r pid; do
	kill -TERM "$pid" 2>/dev/null && echo "stopping pid $pid" || true
done < .pids
rm -f .pids
