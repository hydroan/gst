#!/usr/bin/env bash
# Starts three replicas of the example on this machine, on ports 8081-8083,
# against the MySQL named by MYSQL_HOST/MYSQL_PORT/MYSQL_DATABASE/
# MYSQL_USERNAME/MYSQL_PASSWORD (defaults: config.ini). The first replica
# creates the tables and the others start once it is ready, with migration
# off; see the README for why. Each replica logs under logs/replica<n>;
# stop-local.sh stops them.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p bin
go build -o bin/cluster .

: > .pids
for n in 1 2 3; do
	mkdir -p "logs/replica$n"
	migrate=false
	[ "$n" = 1 ] && migrate=true
	SERVER_PORT="808$n" LOGGER_DIR="logs/replica$n" DATABASE_AUTO_MIGRATE="$migrate" \
		./bin/cluster > "logs/replica$n/stdout.log" 2>&1 &
	echo $! >> .pids
	echo "replica$n: pid $! port 808$n logs logs/replica$n"
	if [ "$n" = 1 ]; then
		for _ in $(seq 1 60); do
			curl -fsS -o /dev/null "http://127.0.0.1:8081/-/readyz" 2>/dev/null && break
			sleep 0.5
		done
	fi
done
