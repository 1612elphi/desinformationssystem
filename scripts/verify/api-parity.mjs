// Diff the Python and Go APIs response by response.
// usage: node scripts/verify/api-parity.mjs <python-base> <go-base>
import { isDeepStrictEqual } from 'node:util';

const [a, b] = process.argv.slice(2);
if (!a || !b) {
	console.error('usage: api-parity.mjs <python-base> <go-base>');
	process.exit(2);
}

const paths = [
	'/health', '/api/stats', '/api/facets', '/api/committees', '/api/people',
	'/api/search', '/api/search?q=durlach', '/api/search?q=Straßenbahn&limit=1',
	'/api/search?q=%22%29%28*', '/api/search?q=haus*%20OR', '/api/search?public=0',
	'/api/search?public=true&type=Antrag', '/api/search?submitter=B90',
	'/api/search?topic=Verkehr', '/api/search?district=Durlach',
	'/api/search?street=kaiserstraße', '/api/search?from=2026-07-10&to=2026-07-30',
	'/api/search?committee=Gemeinderat&offset=1', '/api/search?limit=0',
	'/api/search?limit=201', '/api/search?limit=abc',
	'/api/document/1001', '/api/document/1006', '/api/document/nope',
	'/api/meetings', '/api/meetings?upcoming=true', '/api/meetings?order=asc&limit=2&offset=1',
	'/api/meetings?committee=organisation-gr-3', '/api/meetings?limit=501',
	'/api/meeting/termin-100', '/api/meeting/termin-105', '/api/meeting/nope',
	'/api/committee/organisation-gr-1', '/api/committee/organisation-gr-3', '/api/committee/x',
	'/api/live', '/api/recent', '/api/recent?since=2026-07-02&limit=2',
	'/api/person/12', '/api/person/13', '/api/person/11', '/api/person/99', '/api/person/x1',
	'/api/vorlage/2026/0324', '/api/vorlage/2026/0400', '/api/vorlage/2026/9999', '/api/vorlage/1999/0324',
	'/api/analytics', '/api/feed.xml', '/api/feed.xml?submitter=B90&type=Antrag',
	'/api/meetings.ics', '/api/meetings.ics?committee=Gemeinderat',
	'/api/file/1001', '/api/file/1005', '/api/file/nope',
	'/api/file/1001/version/1', '/api/file/1002/version/1',
];

// DTSTAMP is the request time; feed links carry the request host
const norm = (s) => s.replace(/DTSTAMP:\d{8}T\d{6}Z/g, 'DTSTAMP:x').replace(/\/\/localhost:\d+/g, '//localhost');

async function get(base, p) {
	const r = await fetch(base + p, { redirect: 'manual' });
	const type = (r.headers.get('content-type') || '').split(';')[0];
	const text = await r.text();
	let body = norm(text);
	if (type === 'application/json') body = JSON.parse(text);
	return { status: r.status, type, total: r.headers.get('x-total-count'), location: r.headers.get('location'), body };
}

let failures = 0;
for (const p of paths) {
	const [x, y] = await Promise.all([get(a, p), get(b, p)]);
	// FastAPI 422 bodies are pydantic error lists; only the status must match
	if (x.status === 422 && y.status === 422) continue;
	if (!isDeepStrictEqual(x, y)) {
		failures++;
		console.log(`DIFF ${p}`);
		console.log('  py:', JSON.stringify(x).slice(0, 1500));
		console.log('  go:', JSON.stringify(y).slice(0, 1500));
	}
}
console.log(failures ? `FAILURES ${failures}/${paths.length}` : `ALL PASS ${paths.length}`);
process.exit(failures ? 1 : 0);
