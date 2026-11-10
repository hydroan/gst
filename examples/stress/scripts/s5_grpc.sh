#!/bin/bash
# Scenario 5: the gRPC line against HTTP, the same three operations driven by
# k6 on both, each with a session of its own (a session is bound to the user
# agent that logged in, and k6's gRPC and HTTP clients present different ones).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
source "$SCRIPTS/k6run.sh"
ROWS=${ROWS:-100000}
: > "$OUT/results-k6.tsv"
bash "$SCRIPTS/seed.sh" records "$ROWS" | tee -a "$LOG"
measure_baseline
start_server s5
# Learn the user agent k6's gRPC client sends: one call with a session bound
# to another agent is refused, and the gRPC access log names the agent.
PROBE=$(UA='probe/1.0' login root)
k6 run --quiet --no-usage-report --vus 1 --iterations 1 -e GRPC="$GRPC" -e PROTO_DIR="$STRESS_DIR/pb" -e SESSION="$PROBE" -e MODE=get -e ROWS="$ROWS" "$K6/grpc.js" > /dev/null 2>&1; sleep 2
K6_GRPC_UA=$(grep -o '"user_agent":"[^"]*"' "$STRESS_DIR"/logs/grpc.log | tail -1 | cut -d'"' -f4)
log "k6 gRPC user agent: '$K6_GRPC_UA'"
GSESSION=$(UA="$K6_GRPC_UA" login root); HSESSION=$(UA='Grafana k6/2.3.0' login root)
G=(-e SESSION="$GSESSION" -e ROWS="$ROWS" -e VUS=50 -e PROTO_DIR="$STRESS_DIR/pb")
H=(-e SESSION="$HSESSION" -e ROWS="$ROWS" -e VUS=50)
for mode in get list create; do
  k6_case "s5-grpc-$mode" 15 "$K6/grpc.js" grpc_req_duration "${G[@]}" -e MODE=$mode
  k6_case "s5-http-$mode" 15 "$K6/writes.js" http_req_duration "${H[@]}" -e MODE=$mode
done
stop_server
log "s5 done: records=$(rows records)"
