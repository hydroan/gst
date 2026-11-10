#!/bin/bash
# chain.sh runs the scenarios in order, each into its own out/<name>
# directory, resetting the records table to the row count a scenario wants.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
reset_records() { # N
  $MYSQL -N -e "TRUNCATE TABLE records;" 2>/dev/null
  bash "$SCRIPTS/seed.sh" records "$1" | tee -a "$LOG"
}
run() { # name script [VAR=value ...]
  local name=$1 script=$2; shift 2
  log "=== $name start ($script $*)"
  if env OUT="$STRESS_DIR/out/$name" "$@" bash "$SCRIPTS/$script" > "$STRESS_DIR/out/chain-$name.console" 2>&1; then log "=== $name done"; else log "=== $name FAILED exit $? (see chain-$name.console)"; fi
  pkill -f "$BIN" 2>/dev/null; sleep 1
}
log "chain start"
reset_records 100000
run s2-100k s2_data.sh ROWS=100000
run s7 s7_export.sh ROWS=100000
run s8 s8_tenant.sh
run s9 s9_otel.sh ROWS=100000
run s5 s5_grpc.sh ROWS=100000
reset_records 100000
run s3 s3_writes.sh ROWS=100000
run s4 s4_bodylog.sh ROWS=100000
reset_records 1000000
run s2-1m s2_data.sh ROWS=1000000
reset_records 100000
run s10 s10_overload.sh ROWS=100000
log "CHAIN DONE"
