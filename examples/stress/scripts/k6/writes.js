// HTTP writes against examples/stress with a root session. MODE picks the
// request; each virtual user walks its own slice of the seeded rows so no two
// requests touch the same row at once.
//   update     PUT   /api/products/sku-<n>      full row
//   patch      PATCH /api/products/sku-<n>      {score}
//   delete     DELETE /api/products/del-<n>     soft delete, the pool rows are consumed
//   create     POST  /api/products              fresh unique code per request
//   conflict   POST  /api/products              a taken code, every time
//   upsert     POST  /api/products/upsert       existing codes, overwritten
//   upsertnew  POST  /api/products/upsert       fresh codes, inserted
//   deletemany DELETE /api/products/batch       10 ids per request, consumed
//   get        GET   /api/products/sku-<n>      for the client-overhead reference
//   list       GET   /api/products?category=cat-07&_size=20&_sort_by=created_at
import http from 'k6/http';
import { check } from 'k6';

const BASE = __ENV.BASE;
const SESSION = __ENV.SESSION;
const MODE = __ENV.MODE;
const ROWS = Number(__ENV.ROWS || 100000);      // seeded rows sku-00000001..sku-<ROWS>
const DELSTART = Number(__ENV.DELSTART || 0);   // first row the delete modes consume
const params = { headers: { 'Content-Type': 'application/json', Cookie: `session_id=${SESSION}` } };

function pad(n) { return String(n).padStart(8, '0'); }
// rowOf spreads the virtual users over the rows: VU v, iteration i -> row
// (i * VUS + v) mod ROWS, so the users never meet on a row.
function rowOf(base) { return base + ((__ITER * Number(__ENV.VUS || 50) + (__VU - 1)) % ROWS); }
const body = 'lorem ipsum dolor sit amet, consectetur adipiscing elit. '.repeat(7);

export default function () {
  let res;
  switch (MODE) {
    case 'update': {
      const n = 1 + rowOf(0);
      res = http.put(`${BASE}/api/products/sku-${pad(n)}`, JSON.stringify({ code: pad(n), title: `Widget ${n} updated`, category: `cat-${String(n % 20).padStart(2, '0')}`, status: n % 5, score: (n * 13) % 1000, body }), params);
      break;
    }
    case 'patch': {
      const n = 1 + rowOf(0);
      res = http.patch(`${BASE}/api/products/sku-${pad(n)}`, JSON.stringify({ score: (__ITER * 7) % 1000 }), params);
      break;
    }
    case 'delete': {
      const n = DELSTART + __ITER * Number(__ENV.VUS || 50) + __VU;
      res = http.del(`${BASE}/api/products/del-${pad(n)}`, null, params);
      break;
    }
    case 'deletemany': {
      const ids = [];
      const first = DELSTART + (__ITER * Number(__ENV.VUS || 50) + (__VU - 1)) * 10 + 1;
      for (let k = 0; k < 10; k++) ids.push(`del-${pad(first + k)}`);
      res = http.del(`${BASE}/api/products/batch`, JSON.stringify({ ids }), params);
      break;
    }
    case 'create': {
      const code = `k6-${__VU}-${__ITER}-${Date.now()}`;
      res = http.post(`${BASE}/api/products`, JSON.stringify({ code, title: 'created by k6', category: 'cat-k6', status: 1, score: __ITER % 1000, body }), params);
      break;
    }
    case 'conflict': {
      res = http.post(`${BASE}/api/products`, JSON.stringify({ code: pad(1), title: 'conflict', category: 'cat-00', status: 1, score: 1, body }), params);
      break;
    }
    case 'upsert': {
      const n = 1 + rowOf(0);
      res = http.post(`${BASE}/api/products/upsert`, JSON.stringify({ code: pad(n), title: `Widget ${n} upserted`, category: `cat-${String(n % 20).padStart(2, '0')}`, status: n % 5, score: __ITER % 1000, body }), params);
      break;
    }
    case 'upsertnew': {
      res = http.post(`${BASE}/api/products/upsert`, JSON.stringify({ code: `u6-${__VU}-${__ITER}-${Date.now()}`, title: 'upserted new', category: 'cat-k6', status: 1, score: 1, body }), params);
      break;
    }
    case 'get': {
      const n = 1 + rowOf(0);
      res = http.get(`${BASE}/api/products/sku-${pad(n)}`, params);
      break;
    }
    case 'list': {
      res = http.get(`${BASE}/api/products?category=cat-07&_size=20&_sort_by=created_at`, params);
      break;
    }
    default:
      throw new Error(`unknown MODE ${MODE}`);
  }
  const want = MODE === 'conflict' ? 409 : 200;
  check(res, { ok: (r) => r.status === want });
}
