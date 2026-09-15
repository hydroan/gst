#!/usr/bin/env bash
# Starts three replicas of the example on this machine, on ports 8081-8083,
# against the MySQL named by MYSQL_HOST/MYSQL_PORT/MYSQL_DATABASE/
# MYSQL_USERNAME/MYSQL_PASSWORD (defaults: config.ini). All three start at
# once with auto_migrate on; the framework serializes their table creation.
# Each replica logs under logs/replica<n>; stop-local.sh stops them.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p bin
go build -o bin/cluster .

: > .pids
for n in 1 2 3; do
	mkdir -p "logs/replica$n"
	SERVER_PORT="808$n" LOGGER_DIR="logs/replica$n" \
		./bin/cluster > "logs/replica$n/stdout.log" 2>&1 &
	echo $! >> .pids
	echo "replica$n: pid $! port 808$n logs logs/replica$n"
done
