#!/bin/bash
# Scenario 9: tracing exported for real, to the local OTLP/HTTP sink of
# cmd/otlpsink on port 14318 (a port of its own, so a collector on the OTLP
# default 4318 is left alone), at a 1% and a 100% sampling rate, on the
# filtered list and the get of scenario 2.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
ROWS=${ROWS:-100000}
: > "$OUT/results.tsv"; : > "$OUT/allocs.tsv"
bash "$SCRIPTS/seed.sh" records "$ROWS" | tee -a "$LOG"
"$SINK" > "$OUT/sink.log" 2>&1 & SINK=$!
sleep 1; curl -s http://127.0.0.1:14318/stats | tee -a "$LOG"
measure_baseline
for arg in 0.01 1; do
  curl -s http://127.0.0.1:14318/reset > /dev/null
  start_server "s9-otel-$arg" OTEL_ENABLED=true OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://127.0.0.1:14318/v1/traces OTEL_TRACES_SAMPLER=parentbased_traceidratio OTEL_TRACES_SAMPLER_ARG=$arg
  ROOT=$(login root); C=(-H "Cookie: session_id=$ROOT")
  run_case "s9-list-sample$arg" 15 "$BASE/api/records?category=cat-07&_size=20&_sort_by=created_at" "${C[@]}"
  run_case "s9-get-sample$arg" 15 "$BASE/api/records/rec-00050000" "${C[@]}"
  stop_server
  sleep 6
  log "SINK after sampler $arg: $(curl -s http://127.0.0.1:14318/stats)  hey ok200 total=$(cat "$OUT"/s9-*-sample$arg-*.txt 2>/dev/null | grep -E '^\s*\[200\]' | awk '{s+=$2} END{print s}')"
done
kill $SINK 2>/dev/null; wait $SINK 2>/dev/null
log "s9 done"
