#!/bin/bash
# Scenario 2: reads over real data. ROWS records are seeded (ids
# rec-00000001..), then: a filtered page on the indexed column, the same page
# deep in the result, a filter on the column without an index, the unfiltered
# first page and its 1000th (the count runs over the whole table), the cursor
# feed from the middle of the table, a sort on a column without an index, and
# a get.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
ROWS=${ROWS:-100000}
: > "$OUT/results.tsv"; : > "$OUT/allocs.tsv"
bash "$SCRIPTS/seed.sh" records "$ROWS" | tee -a "$LOG"
log "s2 start: records=$(rows records)"
measure_baseline
start_server s2
ROOT=$(login root); C=(-H "Cookie: session_id=$ROOT")
run_case s2-list-cat 15 "$BASE/api/records?category=cat-07&_size=20&_sort_by=created_at" "${C[@]}"
run_case s2-list-cat-deep 15 "$BASE/api/records?category=cat-07&_size=20&_sort_by=created_at&_page=200" "${C[@]}"
run_case s2-list-status 15 "$BASE/api/records?status=3&_size=20&_sort_by=created_at" "${C[@]}"
run_case s2-list-all 15 "$BASE/api/records?_size=20&_sort_by=created_at" "${C[@]}"
run_case s2-list-all-deep 15 "$BASE/api/records?_size=20&_sort_by=created_at&_page=1000" "${C[@]}"
run_case s2-list-cursor 15 "$BASE/api/records?category=cat-07&_size=20&_cursor_value=rec-00050000&_cursor_next=true" "${C[@]}"
run_case s2-list-sort-noindex 15 "$BASE/api/records?_size=20&_sort_by=score" "${C[@]}"
run_case s2-get 15 "$BASE/api/records/rec-00050000" "${C[@]}"
# CPU and allocation profiles of the filtered list and the get, under load.
profile() { # name url
  hey_run "$OUT/pprof-$1-warmup.txt" 10s "$2" "${C[@]}"; cooldown 15
  hey_run "$OUT/pprof-$1-hey.txt" 60s "$2" "${C[@]}" & local hp=$!
  sleep 5; curl -s -o "$OUT/cpu_$1.pb.gz" "$PPROF/debug/pprof/profile?seconds=30"; curl -s -o "$OUT/allocs_$1.pb.gz" "$PPROF/debug/pprof/allocs?seconds=15"; wait $hp
  log "pprof $1: $(parse_hey "$OUT/pprof-$1-hey.txt")"
}
profile list-cat "$BASE/api/records?category=cat-07&_size=20&_sort_by=created_at"
profile get "$BASE/api/records/rec-00050000"
stop_server
log "s2 done"
