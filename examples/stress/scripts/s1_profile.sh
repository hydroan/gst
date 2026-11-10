#!/bin/bash
# CPU and allocation profiles of the public list and the authenticated list of orders.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
measure_baseline
start_server s1p
ROOT=$(login root)
Q='_size=20&_sort_by=created_at'
profile() { # name url [hey args]
  local name=$1 url=$2; shift 2
  hey_run "$OUT/pprof-$name-warmup.txt" 10s "$url" "$@"; cooldown 15
  hey_run "$OUT/pprof-$name-hey.txt" 60s "$url" "$@" & local hp=$!
  sleep 5; curl -s -o "$OUT/cpu_$name.pb.gz" "$PPROF/debug/pprof/profile?seconds=30"; curl -s -o "$OUT/allocs_$name.pb.gz" "$PPROF/debug/pprof/allocs?seconds=15"; wait $hp
  log "pprof $name: $(parse_hey "$OUT/pprof-$name-hey.txt") cpu=$(cpu_summary "$OUT/pprof-$name-hey.txt")"
}
profile public "$BASE/api/orders/public?$Q"
profile root "$BASE/api/orders?$Q" -H "Cookie: session_id=$ROOT"
stop_server
log "s1 profile done"
