#!/bin/bash
# Scenario 8: the tenant scope. 1M tickets over 10 tenants (5000 per tenant
# and category, the cardinality a category has in 100k records); a member of
# tenant t-03 lists and gets its tickets, beside root making the same
# requests on the records table, which carries no tenant. Root itself is not
# used on tickets: holding the system role it reads across every tenant and
# the scope adds no condition for it.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
: > "$OUT/results.tsv"; : > "$OUT/allocs.tsv"
seed_tickets() { # total rows, 10 tenants; the tenant comes from the row number, the category from its tens, so every tenant holds every category
  local N=$1 have start end s e; have=$($MYSQL -N -e "SELECT COALESCE(MAX(CAST(code AS UNSIGNED)), 0) FROM tickets WHERE code REGEXP '^[0-9]{8}\$';" 2>/dev/null); echo "tickets: seeded up to $have, want $N"; start=$((have+1))
  while [ "$start" -le "$N" ]; do end=$((start+99999)); [ "$end" -gt "$N" ] && end=$N
    $MYSQL -N -e "SET SESSION cte_max_recursion_depth = 200000;
INSERT INTO tickets (id, code, title, category, status, body, tenant_id, created_by, updated_by, created_at, updated_at, deleted_at)
WITH RECURSIVE n(i) AS (SELECT $start UNION ALL SELECT i+1 FROM n WHERE i < $end)
SELECT CONCAT('tk-', LPAD(i, 8, '0')), LPAD(i, 8, '0'), CONCAT('Ticket ', i), CONCAT('cat-', LPAD((i DIV 10) % 20, 2, '0')), i % 5,
       REPEAT('lorem ipsum dolor sit amet, consectetur adipiscing elit. ', 7), CONCAT('t-', LPAD(i % 10, 2, '0')), 'seed', 'seed',
       TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, NULL FROM n;" 2>/dev/null
    start=$((end+1))
  done
  echo "tickets now $(rows tickets)"
}
bash "$SCRIPTS/seed.sh" records 100000 | tee -a "$LOG"
seed_tickets 1000000 | tee -a "$LOG"
measure_baseline
start_server s8
MEMBER=$(TENANT=t-03 login member)
ROOT=$(login root)
log "member session ${MEMBER:0:8} root session ${ROOT:0:8}; tickets of t-03/cat-07: $($MYSQL -N -e "SELECT COUNT(*) FROM tickets WHERE tenant_id='t-03' AND category='cat-07';" 2>/dev/null)"
curl -s -A "$UA" -b "session_id=$MEMBER" "$BASE/api/tickets?category=cat-07&_size=2" | cut -c1-300 | tee -a "$LOG"; echo
run_case s8-tickets-list 15 "$BASE/api/tickets?category=cat-07&_size=20&_sort_by=created_at" -H "Cookie: session_id=$MEMBER"
run_case s8-records-list 15 "$BASE/api/records?category=cat-07&_size=20&_sort_by=created_at" -H "Cookie: session_id=$ROOT"
run_case s8-tickets-get 15 "$BASE/api/tickets/tk-00050003" -H "Cookie: session_id=$MEMBER"
run_case s8-records-get 15 "$BASE/api/records/rec-00050000" -H "Cookie: session_id=$ROOT"
stop_server
log "s8 done"
