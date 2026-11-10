#!/bin/bash
# Scenario 7: the export. Single requests, three of each: a category (5% of
# the rows) and the whole table as CSV, timing the request and watching the
# server's memory while it runs.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
ROWS=${ROWS:-100000}
: > "$OUT/export.tsv"
bash "$SCRIPTS/seed.sh" records "$ROWS" | tee -a "$LOG"
measure_baseline
start_server s7
ROOT=$(login root)
export_once() { # name url
  local name=$1 url=$2 peak=0 r c0 c1 t
  : > "$OUT/$name.rss"
  ( while :; do rss_mb "$SERVER_PID" >> "$OUT/$name.rss"; sleep 0.2; done ) & local sp=$!
  c0=$(cputime_s "$SERVER_PID")
  t=$(curl -s -A "$UA" -b "session_id=$ROOT" -o "$OUT/$name.csv" -w '%{time_total} %{size_download} %{http_code}' "$url")
  c1=$(cputime_s "$SERVER_PID")
  kill "$sp" 2>/dev/null; wait "$sp" 2>/dev/null
  peak=$(sort -n "$OUT/$name.rss" | tail -1); local lines; lines=$(wc -l < "$OUT/$name.csv")
  log "EXPORT $name: time/bytes/http=$t rows=$((lines-1)) server_cpu_s=$(awk -v a="$c0" -v b="$c1" 'BEGIN{printf "%.2f", b-a}') rss_peak=${peak}MB rss_now=$(rss_mb "$SERVER_PID")MB"
  printf '%s\t%s\t%s\t%s\t%s\n' "$name" "$t" "$((lines-1))" "$(awk -v a="$c0" -v b="$c1" 'BEGIN{printf "%.2f", b-a}')" "$peak" >> "$OUT/export.tsv"
}
log "rss at start: $(rss_mb "$SERVER_PID")MB"
for i in 1 2 3; do export_once "s7-cat-$i" "$BASE/api/records/export?_format=csv&category=cat-07&_limit=1000000"; sleep 5; done
for i in 1 2 3; do export_once "s7-all-$i" "$BASE/api/records/export?_format=csv&_limit=1000000"; sleep 10; done
stop_server
log "s7 done"
