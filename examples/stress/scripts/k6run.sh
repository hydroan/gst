#!/bin/bash
# k6_case NAME MIN_COOLDOWN SCRIPT METRIC [k6 -e VAR=val ...]: warmup 10s, ROUNDS x 30s,
# cooldown between; one line per round in results-k6.tsv:
# name round pre_idle rate p50 p90 p99 failed cores us_per_req run_idle logs
# where logs is the ERROR, WARN and slow statement entries the round logged
# (see round_logs).
k6_case() {
  local name=$1 min=$2 script=$3 metric=$4; shift 4
  log "k6 case $name warmup"
  [ -n "${BEFORE_ROUND:-}" ] && $BEFORE_ROUND "$name" warmup
  DUR=10s k6_run "$OUT/$name-warmup" "$script" "$metric" "$@" > "$OUT/$name-warmup.parsed"
  log "k6 case $name warmup: $(cat "$OUT/$name-warmup.parsed")"
  [ -n "${AFTER_ROUND:-}" ] && $AFTER_ROUND "$name" warmup
  cooldown "$min"
  local i pre_idle parsed since logs
  for i in $(seq 1 "$ROUNDS"); do
    [ -n "${BEFORE_ROUND:-}" ] && $BEFORE_ROUND "$name" "r$i"
    pre_idle=$(cpu_idle); since=$(date -u +%Y-%m-%dT%H:%M:%S)
    parsed=$(DUR=30s k6_run "$OUT/$name-r$i" "$script" "$metric" "$@"); logs=$(round_logs "$since")
    log "RESULT $name r$i pre_idle=${pre_idle}% run_idle=$(idle_summary "$OUT/$name-r$i")% rate/p50/p90/p99/failed/cores/us=$parsed logs(error warn slow_sql)=$logs"
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$i" "$pre_idle" "$parsed" "$(idle_summary "$OUT/$name-r$i")" "$logs" >> "$OUT/results-k6.tsv"
    [ -n "${AFTER_ROUND:-}" ] && $AFTER_ROUND "$name" "r$i"
    cooldown "$min"
  done
}
