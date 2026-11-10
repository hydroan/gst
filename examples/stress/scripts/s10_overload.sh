#!/bin/bash
# Scenario 10: overload and steadiness. The filtered list under 50, 200, 500
# and 1000 concurrent connections (one round each), the same list at fixed
# arrival rates from 1000/s up (open loop, 30s a step), then 30 minutes of
# mixed load at 50 users with the server's memory, goroutines and MySQL
# connections sampled every 30s.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
ROWS=${ROWS:-100000}
: > "$OUT/results.tsv"; : > "$OUT/allocs.tsv"
bash "$SCRIPTS/seed.sh" records "$ROWS" | tee -a "$LOG"
measure_baseline
# PART picks a half: sweep (connections and arrival rates) or soak; all when empty.
if [ "${PART:-all}" != soak ]; then
start_server s10
ROOT=$(login root); C=(-H "Cookie: session_id=$ROOT")
ROUNDS=1; ALLOCS=0
for c in 50 200 500 1000; do
  CONC=$c run_case "s10-list-c$c" 20 "$BASE/api/records?category=cat-07&_size=20&_sort_by=created_at" "${C[@]}"
  log "after c=$c: rss $(rss_mb "$SERVER_PID")MB goroutines $(curl -s "$PPROF/debug/pprof/goroutine?debug=1" | head -1) mysql threads $($MYSQL -N -e "SHOW STATUS LIKE 'Threads_connected';" 2>/dev/null | awk '{print $2}')"
done
# The open-loop ladder: a fixed arrival rate for 30s per step, the latency it
# gets and whether k6 had to drop iterations to hold the rate.
: > "$OUT/rate.tsv"
RSESSION=$(UA='Grafana k6/2.3.0' login root)
for rate in 1000 2000 4000 6000 8000 10000 12000; do
  c0=$(cputime_s "$SERVER_PID")
  k6 run --quiet --no-usage-report --summary-export "$OUT/rate-$rate.json" -e BASE="$BASE" -e SESSION="$RSESSION" -e RATE=$rate -e DUR=30s "$K6/rate.js" > "$OUT/rate-$rate.txt" 2>&1
  c1=$(cputime_s "$SERVER_PID")
  line=$(python3 - "$OUT/rate-$rate.json" "$c0" "$c1" <<'PY'
import json, sys
j = json.load(open(sys.argv[1])); m = j["metrics"]; d = m["http_req_duration"]
reqs = m["http_reqs"].get("count", 0); rate = m["http_reqs"].get("rate", 0)
dropped = m.get("dropped_iterations", {}).get("count", 0); vus = m.get("vus_max", {}).get("max", m.get("vus_max", {}).get("value", 0))
failed = m.get("http_req_failed", {}).get("passes", 0)
cpu = float(sys.argv[3]) - float(sys.argv[2])
print(f"{rate:.0f}\t{d['med']:.1f}\t{d['p(90)']:.1f}\t{d['p(99)']:.1f}\t{d['max']:.0f}\t{failed}\t{dropped:.0f}\t{vus:.0f}\t{cpu/30:.2f}\t{(cpu*1e6/reqs) if reqs else 0:.1f}")
PY
)
  log "RATE $rate/s -> achieved/p50/p90/p99/max/failed/dropped/vus_max/cores/us: $line"
  printf '%s\t%s\n' "$rate" "$line" >> "$OUT/rate.tsv"
  cooldown 15
done
stop_server
fi
# The soak.
if [ "${PART:-all}" != sweep ]; then
start_server s10-soak
SESSION=$(UA='Grafana k6/2.3.0' login root)
: > "$OUT/soak-samples.tsv"
( while :; do printf '%s\t%s\t%s\t%s\n' "$(date +%H:%M:%S)" "$(rss_mb "$SERVER_PID")" "$(curl -s "$PPROF/debug/pprof/goroutine?debug=1" | head -1 | awk '{print $4}')" "$($MYSQL -N -e "SHOW STATUS LIKE 'Threads_connected';" 2>/dev/null | awk '{print $2}')" >> "$OUT/soak-samples.tsv"; sleep 30; done ) & SAMP=$!
c0=$(cputime_s "$SERVER_PID")
k6 run --quiet --no-usage-report --summary-export "$OUT/soak.json" --summary-trend-stats "min,avg,med,p(90),p(99),max" --vus 50 --duration "${SOAK:-30m}" -e BASE="$BASE" -e SESSION="$SESSION" -e ROWS="$ROWS" -e VUS=50 "$K6/soak.js" > "$OUT/soak.txt" 2>&1
c1=$(cputime_s "$SERVER_PID")
kill $SAMP 2>/dev/null; wait $SAMP 2>/dev/null
log "SOAK done: server cpu seconds $(awk -v a="$c0" -v b="$c1" 'BEGIN{printf "%.0f", b-a}')"
python3 - "$OUT/soak.json" "$OUT/soak-samples.tsv" <<'PY' | tee -a "$LOG"
import json, sys
j = json.load(open(sys.argv[1])); m = j["metrics"]
d = m["http_req_duration"]; it = m["iterations"]; reqs = m["http_reqs"]
fails = m.get("http_req_failed", {}).get("passes", 0)
print(f"SOAK requests={reqs.get('count',0):.0f} rate={reqs.get('rate',0):.0f}/s iterations/s={it.get('rate',0):.0f} p50={d['med']:.1f}ms p90={d['p(90)']:.1f}ms p99={d['p(99)']:.1f}ms max={d['max']:.0f}ms failed={fails}")
rows = [l.split("\t") for l in open(sys.argv[2]).read().strip().split("\n") if l]
rss = [int(r[1]) for r in rows if r[1].isdigit()]; gor = [int(r[2]) for r in rows if len(r) > 2 and r[2].isdigit()]; thr = [int(r[3]) for r in rows if len(r) > 3 and r[3].strip().isdigit()]
print(f"SOAK rss MB first/max/last={rss[0]}/{max(rss)}/{rss[-1]} goroutines first/max/last={gor[0]}/{max(gor)}/{gor[-1]} mysql threads first/max/last={thr[0]}/{max(thr)}/{thr[-1]}")
PY
stop_server
fi
log "s10 done: records=$(rows records)"
