#!/bin/bash
# build.sh builds the server and the OTLP sink the scenarios run, into out/.
set -e
STRESS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mkdir -p "$STRESS_DIR/out"
go -C "$STRESS_DIR" build -trimpath -o "$STRESS_DIR/out/stresssrv" .
go -C "$STRESS_DIR" build -trimpath -o "$STRESS_DIR/out/otlpsink" ./cmd/otlpsink
ls -la "$STRESS_DIR/out/stresssrv" "$STRESS_DIR/out/otlpsink"
