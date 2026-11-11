#!/bin/bash
# Scenario 6: the association preload. The same page of 20 orders without and
# with ?_expand=lines (10 lines each), then the statements one request issues.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
: > "$OUT/results.tsv"; : > "$OUT/allocs.tsv"
bash "$SCRIPTS/seed.sh" orders 10000 | tee -a "$LOG"
log "s6 start: orders=$(rows orders) lines=$(rows order_lines)"
measure_baseline
start_server s6
ROOT=$(login root)
Q='_size=20&_sort_by=created_at'
run_case s6-plain 15 "$BASE/api/orders?$Q" -H "Cookie: session_id=$ROOT"
run_case s6-expand 15 "$BASE/api/orders?$Q&_expand=lines" -H "Cookie: session_id=$ROOT"
stop_server
# One request of each at debug level: the SQL statements it issues.
start_server s6-sql LOGGER_LEVEL=debug
ROOT=$(login root)
# The first request of a session on a server just started loads the user's
# state from the database into the cache, two statements more; it is made
# here, so the two requests counted below are steady ones.
curl -s -o /dev/null -A "$UA" -b "session_id=$ROOT" "$BASE/api/orders?$Q"
T1=$(curl -s -A "$UA" -b "session_id=$ROOT" "$BASE/api/orders?$Q" | grep -o '"trace_id":"[0-9a-f]*"' | cut -d'"' -f4)
T2=$(curl -s -A "$UA" -b "session_id=$ROOT" "$BASE/api/orders?$Q&_expand=lines" | grep -o '"trace_id":"[0-9a-f]*"' | cut -d'"' -f4)
stop_server
for t in $T1 $T2; do
  n=$(cat "$STRESS_DIR"/logs/*.log | grep -F "\"trace_id\":\"$t\"" | grep -c -F '"sql":')
  log "SQL per request trace=$t statements=$n"
  cat "$STRESS_DIR"/logs/*.log | grep -F "\"trace_id\":\"$t\"" | grep -F '"sql":' | grep -o '"sql":"[^"]*"' | cut -c1-160 | tee -a "$LOG"
done
log "s6 done"
