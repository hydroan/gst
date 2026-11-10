#!/bin/bash
# Scenario 4: the request body log, on (the default) against off, on the
# create and the update of scenario 3.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
source "$SCRIPTS/k6run.sh"
ROWS=${ROWS:-100000}
: > "$OUT/results-k6.tsv"
measure_baseline
W=$K6/writes.js
for mode in false true; do
  start_server "s4-bodylog-$mode" LOGGER_HTTP_BODY_ENABLED=$mode
  SESSION=$(UA='Grafana k6/2.3.0' login root)
  E=(-e SESSION="$SESSION" -e ROWS="$ROWS" -e VUS=50)
  k6_case "s4-update-bodylog-$mode" 15 "$W" http_req_duration "${E[@]}" -e MODE=update
  k6_case "s4-create-bodylog-$mode" 15 "$W" http_req_duration "${E[@]}" -e MODE=create
  stop_server
done
log "s4 done"
