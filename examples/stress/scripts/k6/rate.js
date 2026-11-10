// Open-loop load on the filtered list: RATE requests per second are started
// whatever the server's pace (constant-arrival-rate), so slow answers do not
// slow the clients down and understate the tail latency, as a closed loop of
// N connections does. When the pre-allocated users are all waiting on
// answers, k6 adds users up to MAXVUS and then drops iterations: the dropped
// count is the sign the server fell behind the rate.
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    rate: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE),
      timeUnit: '1s',
      duration: __ENV.DUR || '30s',
      preAllocatedVUs: Number(__ENV.PREVUS || 200),
      maxVUs: Number(__ENV.MAXVUS || 2000),
    },
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(99)', 'max'],
};
const params = { headers: { Cookie: `session_id=${__ENV.SESSION}` } };

export default function () {
  const r = http.get(`${__ENV.BASE}/api/records?category=cat-07&_size=20&_sort_by=created_at`, params);
  check(r, { ok: (x) => x.status === 200 });
}
