#!/bin/bash
# Shared helpers for the stress scenarios: the containers, the server, the
# logins, hey with the measurements (idle, server CPU time, allocations), and
# k6 for the scripted loads. Sourced by every scenario script; OUT names the
# directory a run writes into, BIN the server binary build.sh produced.
SCRIPTS=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
STRESS_DIR=$(cd "$SCRIPTS/.." && pwd)
K6=$SCRIPTS/k6
BIN=${BIN:-$STRESS_DIR/out/stresssrv}
SINK=${SINK:-$STRESS_DIR/out/otlpsink}
BASE=http://127.0.0.1:8080
GRPC=127.0.0.1:8091
PPROF=http://127.0.0.1:10002
OUT=${OUT:-$STRESS_DIR/out/run}
mkdir -p "$OUT"
LOG=$OUT/run.log
MAX_COOLDOWN=${MAX_COOLDOWN:-300}
IDLE_MARGIN=${IDLE_MARGIN:-4}
IDLE_TARGET=${IDLE_TARGET:-60}
ROUNDS=${ROUNDS:-3}
UA=${UA:-hey/0.0.1}
ROOT_PASSWORD=stress-root
MYSQL="docker exec gst-stress-mysql mysql -h127.0.0.1 -uroot -pstress stress"
SRVENV=(DEBUG_PPROF_ENABLED=true DEBUG_PPROF_PORT=10002 LOGGER_HTTP_BODY_ENABLED=false LOGGER_MAX_SIZE=1024 OTEL_ENABLED=false DATABASE_TYPE=mysql MYSQL_HOST=127.0.0.1 MYSQL_PORT=3307 MYSQL_DATABASE=stress MYSQL_USERNAME=root MYSQL_PASSWORD=stress REDIS_ADDR=127.0.0.1:6380 IAM_ROOT_PASSWORD=$ROOT_PASSWORD SEED_PASSWORD=$ROOT_PASSWORD)
JSON='Content-Type: application/json'

log() { printf '%s %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$LOG"; }

cpu_idle() { top -l 2 -s 1 -n 0 2>/dev/null | awk '/^CPU usage/ {v=$7} END {sub(/%/,"",v); printf "%d\n", v+0}'; }

measure_baseline() {
  local sum=0 i v
  for i in 1 2 3; do v=$(cpu_idle); sum=$((sum+v)); sleep 1; done
  BASE_IDLE=$((sum/3)); IDLE_TARGET=$((BASE_IDLE-IDLE_MARGIN))
  [ "$IDLE_TARGET" -gt 95 ] && IDLE_TARGET=95; [ "$IDLE_TARGET" -lt 50 ] && IDLE_TARGET=50
  log "baseline idle ${BASE_IDLE}%, cooldown target idle >= ${IDLE_TARGET}%"
}

cooldown() {
  local min=$1 waited idle ok=0
  sleep "$min"; waited=$min
  while :; do
    idle=$(cpu_idle); waited=$((waited+2))
    if [ "$idle" -ge "$IDLE_TARGET" ]; then ok=$((ok+1)); else ok=0; fi
    [ "$ok" -ge 2 ] && break
    if [ "$waited" -ge "$MAX_COOLDOWN" ]; then log "cooldown ceiling ${MAX_COOLDOWN}s reached, idle ${idle}%"; break; fi
    sleep 3; waited=$((waited+3))
  done
  log "cooldown ${waited}s -> idle ${idle}% (target ${IDLE_TARGET}%)"
}

cputime_s() { ps -o cputime= -p "$1" 2>/dev/null | awk -F: '{ n=NF; s=$n+0; if (n>=2) s+=$(n-1)*60; if (n>=3) s+=$(n-2)*3600; printf "%.2f\n", s }'; }
rss_mb() { ps -o rss= -p "$1" 2>/dev/null | awk '{printf "%.0f\n", $1/1024}'; }

# infra_up starts the MySQL and Redis containers the scenarios use.
infra_up() {
  docker ps -a --format '{{.Names}}' | grep -qx gst-stress-mysql || docker run -d --name gst-stress-mysql -e MYSQL_ROOT_PASSWORD=stress -e MYSQL_DATABASE=stress -p 127.0.0.1:3307:3306 --tmpfs /var/lib/mysql:size=6g mysql:8.4 --skip-log-bin --innodb-buffer-pool-size=2G >/dev/null
  docker ps -a --format '{{.Names}}' | grep -qx gst-stress-redis || docker run -d --name gst-stress-redis -p 127.0.0.1:6380:6379 redis:7-alpine >/dev/null
  docker start gst-stress-mysql gst-stress-redis >/dev/null 2>&1 || true
  local i; for i in $(seq 1 120); do docker exec gst-stress-mysql mysqladmin ping -h127.0.0.1 -uroot -pstress --silent 2>/dev/null && break; sleep 1; done
  log "infra up: mysql $(docker exec gst-stress-mysql mysql -h127.0.0.1 -uroot -pstress -N -e 'SELECT VERSION();' 2>/dev/null) redis $(docker exec gst-stress-redis redis-cli ping)"
}
infra_down() { docker rm -f gst-stress-mysql gst-stress-redis >/dev/null 2>&1; log "infra removed"; }

start_server() { # name env=value...
  local name=$1; shift
  # A server left running would answer in place of the one started here, and
  # every measurement would be of it.
  if curl -s -o /dev/null "$BASE/api/ping"; then log "$BASE already answers: stop the server running there first"; exit 1; fi
  SERVER_NAME=$name; SERVER_BIN=$BIN
  cd "$STRESS_DIR" || exit 1
  mkdir -p "$STRESS_DIR/logs"; local f; for f in "$STRESS_DIR"/logs/*.log; do [ -e "$f" ] && : > "$f"; done
  env "${SRVENV[@]}" "$@" "$BIN" > "$OUT/server-$name.log" 2>&1 &
  SERVER_PID=$!
  local i
  for i in $(seq 1 600); do
    curl -sf -o /dev/null "$BASE/api/ping" && break
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then log "server $name died at start"; tail -20 "$OUT/server-$name.log" | tee -a "$LOG"; exit 1; fi
    sleep 0.2
  done
  curl -sf -o /dev/null "$BASE/api/ping" || { log "server $name not ready"; tail -5 "$OUT/server-$name.log"; exit 1; }
  log "server $name up pid $SERVER_PID rss $(rss_mb "$SERVER_PID")MB ($*)"
}

check_logs() {
  local files errs warns
  files=$(ls "$STRESS_DIR"/logs/*.log 2>/dev/null | grep -v -E "/(access|http_body)(-[^/]*)?\.log$")
  errs=$(cat $files 2>/dev/null | grep -c -F '"level":"ERROR"'); warns=$(cat $files 2>/dev/null | grep -c -F '"level":"WARN"')
  log "logs after $SERVER_NAME: stdout errors=$(grep -c -F '"level":"ERROR"' "$OUT/server-$SERVER_NAME.log") stream errors=$errs warns=$warns rss $(rss_mb "$SERVER_PID")MB"
  if [ "$errs" -gt 0 ] || [ "$warns" -gt 0 ]; then cat $files | grep -E -F -e '"level":"ERROR"' -e '"level":"WARN"' | cut -c1-400 | head -5 | tee -a "$LOG"; fi
}

# round_logs prints how many ERROR entries, WARN entries and slow statement
# warnings the streams wrote since a moment (UTC, 2006-01-02T15:04:05), read
# off the entries' own timestamps. A stream rotates and keeps one backup, so
# the scan check_logs makes as a scenario ends has lost what an early round
# logged; a round is scanned as it ends, while its entries are still in the
# file or its backup, which SRVENV sizes at 1 GB each for that: a round
# writes a few hundred MB to its busiest stream. The slow statement warnings
# are counted apart from the other warnings: the method lets them be. An
# entry is on disk within a second of being written, hence the wait.
round_logs() { # since
  sleep 2
  local files; files=$(ls "$STRESS_DIR"/logs/*.log 2>/dev/null | grep -v -E "/(access|http_body)(-[^/]*)?\.log$")
  cat $files 2>/dev/null | grep -a -F -e '"level":"ERROR"' -e '"level":"WARN"' | awk -v since="$1" '
    match($0, /"ts":"[^"]*"/) && substr($0, RSTART + 6, 19) >= since {
      if ($0 ~ /"level":"ERROR"/) e++; else if ($0 ~ /"msg":"slow sql detected"/) s++; else w++
    }
    END { printf "%d %d %d\n", e, w, s }'
}

stop_server() {
  check_logs
  kill -TERM "$SERVER_PID" 2>/dev/null
  local i; for i in $(seq 1 150); do kill -0 "$SERVER_PID" 2>/dev/null || break; sleep 0.2; done
  kill -0 "$SERVER_PID" 2>/dev/null && { log "server still alive after 30s, SIGKILL"; kill -KILL "$SERVER_PID"; }
  wait "$SERVER_PID" 2>/dev/null
  for i in $(seq 1 50); do curl -s -o /dev/null "$BASE/api/ping" || break; sleep 0.2; done
  log "server $SERVER_NAME stopped"
}

# login prints the session id of an account; the session is bound to UA, and
# acts in the tenant TENANT names when it is set.
login() { # username [password]
  curl -s -c - -A "$UA" -X POST "$BASE/api/login" -H "$JSON" -d "{\"username\":\"$1\",\"password\":\"${2:-$ROOT_PASSWORD}\"${TENANT:+,\"tenant_id\":\"$TENANT\"}}" | awk '$6 == "session_id" {print $7}'
}

parse_hey() { # prints: qps p50ms p90ms p99ms non200 ok200 total_s
  awk '
    /Requests\/sec:/ {qps=$2}
    /^  Total:/ {total=$2}
    /^  50%%? in/ {p50=$3} /^  90%%? in/ {p90=$3} /^  99%%? in/ {p99=$3}
    /Status code distribution:/ {insc=1; inerr=0; next}
    /Error distribution:/ {insc=0; inerr=1; next}
    insc && /^  \[/ { code=$1; gsub(/[\[\]]/,"",code); n=$2; if (code!="200") non200+=n; else ok+=n }
    inerr && /^  \[/ { n=$1; gsub(/[\[\]]/,"",n); non200+=n }
    END { printf "%.0f %.1f %.1f %.1f %d %d %.2f\n", qps, p50*1000, p90*1000, p99*1000, non200+0, ok+0, total }' "$1"
}

hey_run() { # outfile duration url [hey args...]
  local out=$1 dur=$2 url=$3; shift 3
  : > "$out.idle"; : > "$out.ps"
  ( n=0; while :; do cpu_idle >> "$out.idle"; n=$((n+1)); [ $((n % 10)) -eq 1 ] && { date +%H:%M:%S; ps -Aro %cpu=,comm= | head -6; } >> "$out.ps"; sleep 1; done ) & local sp=$!
  local c0 c1; c0=$(cputime_s "$SERVER_PID")
  hey -z "$dur" -c "${CONC:-50}" "$@" "$url" > "$out" 2>&1
  c1=$(cputime_s "$SERVER_PID")
  kill "$sp" 2>/dev/null; wait "$sp" 2>/dev/null
  echo "$c0 $c1" > "$out.cpu"
}
idle_summary() { awk 'NR==1{m=$1} {s+=$1; if($1<m)m=$1} END{if(NR) printf "%d/%d", m, s/NR; else printf "-/-"}' "$1.idle" 2>/dev/null; }
cpu_summary() { # cores us_per_request
  local c0 c1 parsed qps total; read -r c0 c1 < "$1.cpu"; parsed=$(parse_hey "$1"); qps=$(echo "$parsed" | awk '{print $1}'); total=$(echo "$parsed" | awk '{print $7}')
  awk -v c0="$c0" -v c1="$c1" -v qps="$qps" -v total="$total" 'BEGIN { d=c1-c0; if (total>0 && qps>0) printf "%.2f %.1f\n", d/total, d*1e6/(qps*total); else print "0 0" }'
}

# alloc_profile writes the process's cumulative allocation profile to a file.
# The profile trails the allocations by up to two collections, so two are run
# first.
alloc_profile() { # file
  curl -s -o /dev/null "$PPROF/debug/pprof/heap?gc=1"; curl -s -o "$1" "$PPROF/debug/pprof/heap?gc=1"
}
# alloc_delta prints what the process allocated between two of those
# profiles, in bytes (alloc_space) or in objects (alloc_objects).
alloc_delta() { # sample_index before after
  go tool pprof -top -unit=bytes -sample_index="$1" -base "$2" "$3" 2>/dev/null | awk '/Showing nodes accounting for/ { v=$(NF-1); gsub(/[^0-9]/,"",v); print v; exit }'
}
# alloc_case measures what one request of a variant allocates: hey completes
# a fixed number of requests, 20 s of them at the rate given, no fewer than
# ALLOC_MIN_REQUESTS and no more than the million hey's report counts,
# between two readings of the cumulative allocation profile, and the
# difference is divided by the requests completed. The line goes to
# allocs.tsv: the case, KB and objects per request, the requests.
alloc_case() { # name rate url [hey args...]
  local name=$1 rate=$2 url=$3 n parsed count space objs kb per; shift 3
  n=$(awk -v q="$rate" -v c="${CONC:-50}" -v m="${ALLOC_MIN_REQUESTS:-20000}" 'BEGIN { n = q * 20; if (n < m) n = m; if (n > 1000000) n = 1000000; print int(n / c) * c }')
  alloc_profile "$OUT/$name-allocs-before.pb.gz"
  hey -n "$n" -c "${CONC:-50}" "$@" "$url" > "$OUT/$name-allocs.txt" 2>&1
  alloc_profile "$OUT/$name-allocs-after.pb.gz"
  parsed=$(parse_hey "$OUT/$name-allocs.txt"); count=$(echo "$parsed" | awk '{print $5 + $6}')
  space=$(alloc_delta alloc_space "$OUT/$name-allocs-before.pb.gz" "$OUT/$name-allocs-after.pb.gz")
  objs=$(alloc_delta alloc_objects "$OUT/$name-allocs-before.pb.gz" "$OUT/$name-allocs-after.pb.gz")
  kb=$(awk -v b="$space" -v n="$count" 'BEGIN { if (n > 0) printf "%.1f", b / 1024 / n; else print 0 }'); per=$(awk -v o="$objs" -v n="$count" 'BEGIN { if (n > 0) printf "%.0f", o / n; else print 0 }')
  log "ALLOCS $name: ${kb}KB/req ${per}objs/req ($count requests, non200 $(echo "$parsed" | awk '{print $5}'))"
  printf '%s\t%s\t%s\t%s\n' "$name" "$kb" "$per" "$count" >> "$OUT/allocs.tsv"
}

# run_case: warmup 10s -> cooldown -> ROUNDS x 30s, each with the ERROR, WARN
# and slow statement entries the round logged (see round_logs) -> the
# allocations of one request (see alloc_case). Results go to results.tsv (one
# line per round) and allocs.tsv (one line per case).
run_case() { # name min_cooldown url [hey args...]
  local name=$1 min=$2 url=$3; shift 3
  log "case $name warmup"
  hey_run "$OUT/$name-warmup.txt" 10s "$url" "$@"
  log "case $name warmup: $(parse_hey "$OUT/$name-warmup.txt")"
  [ -n "${AFTER_ROUND:-}" ] && $AFTER_ROUND "$name" warmup
  cooldown "$min"
  local i pre_idle parsed cpu since logs
  for i in $(seq 1 "$ROUNDS"); do
    pre_idle=$(cpu_idle); since=$(date -u +%Y-%m-%dT%H:%M:%S)
    hey_run "$OUT/$name-r$i.txt" 30s "$url" "$@"
    parsed=$(parse_hey "$OUT/$name-r$i.txt"); cpu=$(cpu_summary "$OUT/$name-r$i.txt"); logs=$(round_logs "$since")
    log "RESULT $name r$i pre_idle=${pre_idle}% run_idle=$(idle_summary "$OUT/$name-r$i.txt")% cpu(cores us/req)=$cpu logs(error warn slow_sql)=$logs $parsed"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$i" "$pre_idle" "$parsed" "$(idle_summary "$OUT/$name-r$i.txt")" "$cpu" "$logs" >> "$OUT/results.tsv"
    [ -n "${AFTER_ROUND:-}" ] && $AFTER_ROUND "$name" "r$i"
    cooldown "$min"
  done
  if [ "${ALLOCS:-1}" = 1 ]; then
    alloc_case "$name" "$(echo "$parsed" | awk '{print $1}')" "$url" "$@"
    [ -n "${AFTER_ROUND:-}" ] && $AFTER_ROUND "$name" allocs
    cooldown "$min"
  fi
}

# k6_run runs a k6 script for DUR seconds with VUS virtual users and prints:
# rate p50ms p90ms p99ms failed total_s (http or grpc, by METRIC).
k6_run() { # outprefix script metric(http_req_duration|grpc_req_duration) [k6 -e VAR=val ...]
  local out=$1 script=$2 metric=$3; shift 3
  : > "$out.idle"; : > "$out.ps"
  ( n=0; while :; do cpu_idle >> "$out.idle"; n=$((n+1)); [ $((n % 10)) -eq 1 ] && { date +%H:%M:%S; ps -Aro %cpu=,comm= | head -6; } >> "$out.ps"; sleep 1; done ) & local sp=$!
  local c0 c1; c0=$(cputime_s "$SERVER_PID")
  k6 run --quiet --no-usage-report --summary-export "$out.json" --summary-trend-stats "min,avg,med,p(90),p(99),max" --vus "${CONC:-50}" --duration "${DUR:-30s}" -e BASE="$BASE" -e GRPC="$GRPC" "$@" "$script" > "$out.txt" 2>&1
  c1=$(cputime_s "$SERVER_PID")
  kill "$sp" 2>/dev/null; wait "$sp" 2>/dev/null
  echo "$c0 $c1" > "$out.cpu"
  python3 - "$out.json" "$metric" "$out.cpu" "${DUR:-30s}" <<'PY'
import json, sys
j = json.load(open(sys.argv[1])); m = sys.argv[2]
d = j["metrics"][m]
it = j["metrics"]["iterations"]; rate = it.get("rate", 0)
failed = 0
for k in ("http_req_failed", "grpc_req_failed", "checks"):
    if k in j["metrics"]:
        v = j["metrics"][k]
        if k == "checks": failed = v.get("fails", 0)
        else: failed = v.get("passes", 0); break
c0, c1 = map(float, open(sys.argv[3]).read().split()); dur = float(sys.argv[4].rstrip("s"))
cores = (c1 - c0) / dur; us = (c1 - c0) * 1e6 / (rate * dur) if rate else 0
print(f"{rate:.0f} {d['med']:.1f} {d['p(90)']:.1f} {d['p(99)']:.1f} {failed} {cores:.2f} {us:.1f}")
PY
}

rows() { $MYSQL -N -e "SELECT COUNT(*) FROM $1;" 2>/dev/null; }
