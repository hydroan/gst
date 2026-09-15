#!/usr/bin/env bash
# Stops the replicas run-local.sh started, gracefully: SIGTERM starts the
# framework's shutdown, which releases the leases the replica holds.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -f .pids ] || { echo "nothing to stop"; exit 0; }
pids=$(cat .pids)
for pid in $pids; do
	kill -TERM "$pid" 2>/dev/null && echo "stopping pid $pid" || true
done

# The shutdown takes the drain window plus whatever round is in flight; wait
# for it, bounded by the framework's longest shutdown, so that the ports are
# free again when this script returns.
for _ in $(seq 1 140); do
	running=0
	for pid in $pids; do
		kill -0 "$pid" 2>/dev/null && running=1
	done
	[ "$running" = 0 ] && break
	sleep 0.5
done
for pid in $pids; do
	kill -0 "$pid" 2>/dev/null && echo "pid $pid is still shutting down" || true
done
rm -f .pids
