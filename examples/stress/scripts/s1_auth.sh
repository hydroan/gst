#!/bin/bash
# Scenario 1: what a session costs. The same list of 20 orders out of 10k,
# public, as root (system role) and as reader (a plain role), beside ping.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
: > "$OUT/results.tsv"; : > "$OUT/allocs.tsv"
bash "$SCRIPTS/seed.sh" orders 10000 | tee -a "$LOG"
log "s1 start: orders=$(rows orders) lines=$(rows order_lines)"
measure_baseline
start_server s1
ROOT=$(login root); READER=$(login reader)
Q='_size=20&_sort_by=created_at'
run_case s1-ping 10 "$BASE/api/ping"
run_case s1-public 15 "$BASE/api/orders/public?$Q"
run_case s1-root 15 "$BASE/api/orders?$Q" -H "Cookie: session_id=$ROOT"
run_case s1-reader 15 "$BASE/api/orders?$Q" -H "Cookie: session_id=$READER"
stop_server
log "s1 done"
