#!/bin/bash
# Text reports from the pprof profiles a scenario collected: analyze.sh OUTDIR name...
STRESS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT=$1; shift; BIN=${BIN:-$STRESS_DIR/out/stresssrv}; R=$OUT/reports; mkdir -p "$R"
for n in "$@"; do
  [ -f "$OUT/cpu_$n.pb.gz" ] || { echo "no cpu_$n"; continue; }
  go tool pprof -top -cum "$BIN" "$OUT/cpu_$n.pb.gz" > "$R/cpu_$n-cum.txt" 2>&1
  go tool pprof -top "$BIN" "$OUT/cpu_$n.pb.gz" > "$R/cpu_$n-flat.txt" 2>&1
  go tool pprof -top -focus='hydroan/gst' "$BIN" "$OUT/cpu_$n.pb.gz" > "$R/cpu_$n-gst.txt" 2>&1
  [ -f "$OUT/allocs_$n.pb.gz" ] && go tool pprof -top -sample_index=alloc_space "$BIN" "$OUT/allocs_$n.pb.gz" > "$R/allocs_$n-space.txt" 2>&1
  [ -f "$OUT/allocs_$n.pb.gz" ] && go tool pprof -top -cum -sample_index=alloc_space "$BIN" "$OUT/allocs_$n.pb.gz" > "$R/allocs_$n-cum.txt" 2>&1
  echo "$n: $(sed -n '4p' "$R/cpu_$n-flat.txt")"
done
