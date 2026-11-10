#!/bin/bash
# Scenario 3: real writes, k6 driven so every request touches a row of its
# own. The deletes consume rows: each round gets a pool of its own, hard
# deleted afterwards so the table does not grow.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
source "$SCRIPTS/k6run.sh"
ROWS=${ROWS:-100000}
# MODES picks the variants to run, all when empty: MODES="delete deletemany".
want() { [ -z "${MODES:-}" ] || [[ " $MODES " == *" $1 "* ]]; }
: > "$OUT/results-k6.tsv"
bash "$SCRIPTS/seed.sh" records "$ROWS" | tee -a "$LOG"
log "s3 start: records=$(rows records)"
measure_baseline
start_server s3
SESSION=$(UA='Grafana k6/2.3.0' login root)
E=(-e SESSION="$SESSION" -e ROWS="$ROWS" -e VUS=50)
W=$K6/writes.js
want update && k6_case s3-update 15 "$W" http_req_duration "${E[@]}" -e MODE=update
want patch && k6_case s3-patch 15 "$W" http_req_duration "${E[@]}" -e MODE=patch
want upsert && k6_case s3-upsert 15 "$W" http_req_duration "${E[@]}" -e MODE=upsert
want upsertnew && k6_case s3-upsertnew 15 "$W" http_req_duration "${E[@]}" -e MODE=upsertnew
want create && k6_case s3-create 15 "$W" http_req_duration "${E[@]}" -e MODE=create
want conflict && k6_case s3-conflict 15 "$W" http_req_duration "${E[@]}" -e MODE=conflict
# A pool of rows per delete round (ids del-<n>), hard deleted afterwards:
# a round consumes one row per request, or ten for deletemany, so the pool
# is sized per mode (POOL, set before each case).
POOL=1200000
pool_fill() { # start
  local start=$1 end=$(( $1 + POOL - 1 )) s e
  s=$start
  while [ "$s" -le "$end" ]; do e=$((s+99999)); [ "$e" -gt "$end" ] && e=$end
    $MYSQL -N -e "SET SESSION cte_max_recursion_depth = 200000;
INSERT INTO records (id, code, title, category, status, score, body, created_by, updated_by, created_at, updated_at, deleted_at)
WITH RECURSIVE n(i) AS (SELECT $s UNION ALL SELECT i+1 FROM n WHERE i < $e)
SELECT CONCAT('del-', LPAD(i, 8, '0')), CONCAT('d', LPAD(i, 8, '0')), CONCAT('Pool ', i), 'cat-del', 0, 0, 'pool', 'seed', 'seed', NOW(3), NOW(3), NULL FROM n;" 2>/dev/null
    s=$((e+1))
  done
}
pool_drop() { $MYSQL -N -e "DELETE FROM records WHERE id LIKE 'del-%';" 2>/dev/null; }
DELPOS=0
before_delete() { pool_drop; pool_fill 1; DELPOS=0; log "pool ready: $(rows records) rows"; }
BEFORE_ROUND=before_delete
# delete: VU v iteration i -> del-(i*VUS+v); deletemany: 10 ids per request.
POOL=1200000; want delete && k6_case s3-delete 15 "$W" http_req_duration "${E[@]}" -e MODE=delete -e DELSTART=0
POOL=3000000; want deletemany && k6_case s3-deletemany 15 "$W" http_req_duration "${E[@]}" -e MODE=deletemany -e DELSTART=0
BEFORE_ROUND=
pool_drop
stop_server
log "s3 done: records=$(rows records)"
