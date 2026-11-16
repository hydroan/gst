#!/bin/bash
# seed.sh products N   — fills products up to N rows (chunks of 100k), ids sku-00000001..
# seed.sh orders N    — fills orders up to N rows with 10 lines each
# seed.sh truncate    — empties products, orders, order_lines
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
sql() { $MYSQL -N -e "$1" 2>&1 | grep -v "Using a password"; }
case $1 in
truncate) sql "TRUNCATE TABLE products; TRUNCATE TABLE orders; TRUNCATE TABLE order_lines;"; echo "truncated";;
products)
  N=$2; have=$(sql "SELECT COALESCE(MAX(CAST(code AS UNSIGNED)), 0) FROM products WHERE code REGEXP '^[0-9]{8}$';"); echo "products: seeded up to $have, want $N"
  start=$((have+1)); t0=$(date +%s)
  while [ "$start" -le "$N" ]; do
    end=$((start+99999)); [ "$end" -gt "$N" ] && end=$N
    sql "SET SESSION cte_max_recursion_depth = 200000;
INSERT INTO products (id, code, title, category, status, score, body, created_by, updated_by, created_at, updated_at, deleted_at)
WITH RECURSIVE n(i) AS (SELECT $start UNION ALL SELECT i+1 FROM n WHERE i < $end)
SELECT CONCAT('sku-', LPAD(i, 8, '0')), LPAD(i, 8, '0'), CONCAT('Widget ', i), CONCAT('cat-', LPAD(i % 20, 2, '0')), i % 5, (i * 7919) % 1000,
       REPEAT('lorem ipsum dolor sit amet, consectetur adipiscing elit. ', 7), 'seed', 'seed',
       TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, NULL FROM n;"
    start=$((end+1))
  done
  echo "products now $(rows products) rows in $(( $(date +%s) - t0 ))s";;
orders)
  N=$2; have=$(sql "SELECT COALESCE(MAX(CAST(number AS UNSIGNED)), 0) FROM orders WHERE number REGEXP '^[0-9]{8}$';"); echo "orders: seeded up to $have, want $N"
  start=$((have+1)); t0=$(date +%s)
  while [ "$start" -le "$N" ]; do
    end=$((start+9999)); [ "$end" -gt "$N" ] && end=$N
    sql "SET SESSION cte_max_recursion_depth = 20000;
INSERT INTO orders (id, number, customer, total, created_by, updated_by, created_at, updated_at, deleted_at)
WITH RECURSIVE n(i) AS (SELECT $start UNION ALL SELECT i+1 FROM n WHERE i < $end)
SELECT CONCAT('ord-', LPAD(i, 8, '0')), LPAD(i, 8, '0'), CONCAT('cust-', LPAD(i % 500, 3, '0')), (i * 31) % 10000, 'seed', 'seed',
       TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, NULL FROM n;
INSERT INTO order_lines (id, order_id, product, qty, price, created_by, updated_by, created_at, updated_at, deleted_at)
WITH RECURSIVE n(i) AS (SELECT $start UNION ALL SELECT i+1 FROM n WHERE i < $end),
     j(k) AS (SELECT 1 UNION ALL SELECT k+1 FROM j WHERE k < 10)
SELECT CONCAT('line-', LPAD(i, 8, '0'), '-', LPAD(k, 2, '0')), CONCAT('ord-', LPAD(i, 8, '0')), CONCAT('prod-', LPAD(k, 2, '0')), k, (i + k) % 100, 'seed', 'seed',
       TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, TIMESTAMP('2025-01-01 00:00:00') + INTERVAL i SECOND, NULL FROM n, j;"
    start=$((end+1))
  done
  echo "orders now $(rows orders) rows, lines $(rows order_lines) in $(( $(date +%s) - t0 ))s";;
*) echo "usage: seed.sh products N | orders N | truncate"; exit 1;;
esac
