#!/usr/bin/env bash
# Keeps the log of every replica in a local directory, one file per container,
# for as long as it runs: a deleted pod or a restarted container takes its log
# with it, and a scenario is checked against the logs of every replica that
# took part in it. Start it in the background before the scenarios, and stop
# it once they are checked:
#
#	examples/cluster/scripts/collect-logs.sh cluster-logs >/dev/null 2>&1 &
#
# Each file holds the whole log of one container. A container restarting just
# as its follow starts can be followed twice, and running the script again on
# the same directory follows every running container again: read the files
# through sort -u, which drops the repeats — no two entries share their
# instance and their nanosecond timestamp.
set -uo pipefail

dir=${1:-cluster-logs}
mkdir -p "$dir"
trap 'exit' INT TERM
trap 'kill $(jobs -p) 2>/dev/null' EXIT

followed=" "
while true; do
  while read -r pod cid started; do
    # Only a running container has a log to follow.
    [ -n "$started" ] || continue
    id=${cid##*/}
    id=${id:0:12}
    case "$followed" in *" $pod.$id "*) continue ;; esac
    followed="$followed$pod.$id "
    kubectl -n gst-cluster logs -f "$pod" -c cluster >>"$dir/$pod.$id.log" 2>/dev/null &
  done < <(kubectl -n gst-cluster get pods -l app.kubernetes.io/name=cluster \
    -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.containerStatuses[0].containerID}{" "}{.status.containerStatuses[0].state.running.startedAt}{"\n"}{end}' 2>/dev/null)
  sleep 2
done
