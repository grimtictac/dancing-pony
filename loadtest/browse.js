// Browsing load test: what most traffic looks like. A logged-in customer
// lists the menu (50%), searches it (30%) or opens a dish (20%), at a fixed
// arrival rate, so a slow server shows up as latency and dropped
// iterations rather than as a lower request rate.
//
//   make load-up && make load-test          (RATE=200 DURATION=1m to change)
//   make load-test-prod                     (against https://silson.dev)
import http from 'k6/http';
import { check } from 'k6';

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:18083';
const MENU = `${BASE}/restaurants/${__ENV.RESTAURANT || 'load-test'}/dishes`;

// The bar for a 1 vCPU / 512 MB "Shire server". k6 exits non-zero if any is
// missed. Against a remote server the client's network latency would
// dominate these, so REMOTE=1 keeps only the error rate; server-side latency
// comes from the app's own metrics instead (loadtest/latency.py).
const thresholds = { http_req_failed: ['rate<0.01'] };
if (!__ENV.REMOTE) {
  thresholds['http_req_duration{name:list}'] = ['p(95)<250'];
  thresholds['http_req_duration{name:search}'] = ['p(95)<250'];
  thresholds['http_req_duration{name:dish}'] = ['p(95)<100'];
}

export const options = {
  scenarios: {
    browse: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 100),
      timeUnit: '1s',
      duration: __ENV.DURATION || '30s',
      preAllocatedVUs: 50,
      maxVUs: 300,
    },
  },
  thresholds,
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max'],
};

export function setup() {
  const login = http.post(
    `${BASE}/auth/login`,
    JSON.stringify({ email: 'frodo@shire.example', password: 'pony-demo-password' }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (login.status !== 200) throw new Error(`login failed: ${login.status} ${login.body}`);
  const token = login.json('token');
  const page = http.get(`${MENU}?limit=100&sort=-price`, { headers: { Authorization: `Bearer ${token}` } });
  return { token, ids: page.json().map((d) => d.id) };
}

const terms = ['mushroom', 'mushrom', 'pie', 'honey cake', 'stew', 'lembas', 'potato soup'];
const sorts = ['', 'name', '-name', 'price', '-price'];
const pick = (a) => a[Math.floor(Math.random() * a.length)];

export default function ({ token, ids }) {
  const get = (url, name) => http.get(url, { headers: { Authorization: `Bearer ${token}` }, tags: { name } });
  const r = Math.random();
  let res;
  if (r < 0.5) {
    res = get(`${MENU}?limit=20&sort=${pick(sorts)}`, 'list');
  } else if (r < 0.8) {
    res = get(`${MENU}?limit=20&q=${encodeURIComponent(pick(terms))}`, 'search');
  } else {
    res = get(`${MENU}/${pick(ids)}`, 'dish');
  }
  check(res, { 'status 200': (x) => x.status === 200 });
}
