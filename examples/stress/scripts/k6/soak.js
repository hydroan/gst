// A mixed load for the soak: per iteration one list, two gets, one update
// and, every hundredth iteration, one create; a root session.
import http from 'k6/http';
import { check } from 'k6';

const BASE = __ENV.BASE;
const ROWS = Number(__ENV.ROWS || 100000);
const params = { headers: { 'Content-Type': 'application/json', Cookie: `session_id=${__ENV.SESSION}` } };
const body = 'lorem ipsum dolor sit amet, consectetur adipiscing elit. '.repeat(7);
function pad(n) { return String(n).padStart(8, '0'); }

export default function () {
  const n = 1 + ((__ITER * Number(__ENV.VUS || 50) + (__VU - 1)) % ROWS);
  const r1 = http.get(`${BASE}/api/records?category=cat-${String(n % 20).padStart(2, '0')}&_size=20&_sort_by=created_at`, params);
  const r2 = http.get(`${BASE}/api/records/rec-${pad(n)}`, params);
  const r3 = http.get(`${BASE}/api/records/rec-${pad(1 + ((n + 7) % ROWS))}`, params);
  const r4 = http.put(`${BASE}/api/records/rec-${pad(n)}`, JSON.stringify({ code: pad(n), title: `Record ${n} soak`, category: `cat-${String(n % 20).padStart(2, '0')}`, status: n % 5, score: __ITER % 1000, body }), params);
  check(r1, { list: (r) => r.status === 200 }); check(r2, { get: (r) => r.status === 200 }); check(r3, { get2: (r) => r.status === 200 }); check(r4, { update: (r) => r.status === 200 });
  if (__ITER % 100 === 0) {
    const r5 = http.post(`${BASE}/api/records`, JSON.stringify({ code: `soak-${__VU}-${__ITER}-${Date.now()}`, title: 'soak', category: 'cat-k6', status: 1, score: 1, body }), params);
    check(r5, { create: (r) => r.status === 200 });
  }
}
