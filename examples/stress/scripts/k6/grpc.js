// gRPC calls on stress.RecordService with a session in the authorization
// metadata. MODE: get | list | create.
import grpc from 'k6/net/grpc';
import { check } from 'k6';

const client = new grpc.Client();
client.load([__ENV.PROTO_DIR], 'record.proto');
const MODE = __ENV.MODE;
const ROWS = Number(__ENV.ROWS || 100000);
const body = 'lorem ipsum dolor sit amet, consectetur adipiscing elit. '.repeat(7);
function pad(n) { return String(n).padStart(8, '0'); }

export default function () {
  if (__ITER === 0) client.connect(__ENV.GRPC, { plaintext: true });
  const meta = { metadata: { authorization: `Bearer ${__ENV.SESSION}` } };
  let res;
  switch (MODE) {
    case 'get': {
      const n = 1 + ((__ITER * Number(__ENV.VUS || 50) + (__VU - 1)) % ROWS);
      res = client.invoke('stress.RecordService/GetRecord', { id: `rec-${pad(n)}` }, meta);
      break;
    }
    case 'list':
      res = client.invoke('stress.RecordService/ListRecord', { filters: [{ field: 'category', values: ['cat-07'] }], sort_by: ['created_at'], size: 20 }, meta);
      break;
    case 'create':
      res = client.invoke('stress.RecordService/CreateRecord', { record: { code: `g6-${__VU}-${__ITER}-${Date.now()}`, title: 'created over grpc', category: 'cat-k6', status: 1, score: 1, body } }, meta);
      break;
    default:
      throw new Error(`unknown MODE ${MODE}`);
  }
  check(res, { ok: (r) => r && r.status === grpc.StatusOK });
}
