#!/bin/bash
# chain.sh runs the ten scenarios, each into its own out/<name> directory, on
# containers it starts anew. A scenario that states a table size starts from
# a records table refilled to exactly that size, and the tickets of scenario
# 8 are emptied once it ends: MySQL keeps its data in a 6 GB tmpfs, which the
# rows the write scenarios leave behind fill otherwise, and a full tmpfs
# fails every write after it. The chain stops when a refill does not leave
# the size asked for: a scenario run on another table size measures
# something else.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
reset_records() { # N
  $MYSQL -N -e "TRUNCATE TABLE records;" 2>&1 | grep -v "Using a password" | tee -a "$LOG"
  bash "$SCRIPTS/seed.sh" records "$1" | tee -a "$LOG"
  local n; n=$(rows records)
  [ "$n" = "$1" ] || { log "records holds $n rows, not $1: stopping"; exit 1; }
}
run() { # name script [VAR=value ...]
  local name=$1 script=$2; shift 2
  log "=== $name start ($script $*)"
  if env OUT="$STRESS_DIR/out/$name" "$@" bash "$SCRIPTS/$script" > "$STRESS_DIR/out/chain-$name.console" 2>&1; then log "=== $name done"; else log "=== $name FAILED exit $? (see chain-$name.console)"; fi
  pkill -f "$BIN" 2>/dev/null; sleep 1
}
log "chain start"
infra_down; infra_up
# The first start creates the tables and seeds the accounts.
start_server init; stop_server
run s1 s1_auth.sh
run s1p s1_profile.sh
run s6 s6_expand.sh
reset_records 100000
run s2-100k s2_data.sh ROWS=100000
run s7 s7_export.sh ROWS=100000
run s8 s8_tenant.sh
$MYSQL -N -e "TRUNCATE TABLE tickets;" 2>&1 | grep -v "Using a password" | tee -a "$LOG"
run s9 s9_otel.sh ROWS=100000
run s5 s5_grpc.sh ROWS=100000
reset_records 100000
run s3 s3_writes.sh ROWS=100000
reset_records 100000
run s4 s4_bodylog.sh ROWS=100000
reset_records 1000000
# No allocations on 1M rows: they are the ones of 100k rows, and counting
# them over 20000 requests takes minutes at the rates these lists reach.
run s2-1m s2_data.sh ROWS=1000000 ALLOCS=0
reset_records 100000
run s10 s10_overload.sh ROWS=100000
log "CHAIN DONE"
