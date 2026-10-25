const goBase = process.argv[2] ?? 'http://127.0.0.1:4380';
const pyBase = process.argv[3] ?? 'https://dis.delphi.tools';
const paths = [
	'/api/speeches/analytics',
	'/api/speeches/analytics?meeting=termin-10574',
	'/api/meeting/termin-10574/speeches',
	'/api/meeting/termin-10574/speeches?raw=true',
	'/api/meeting/termin-10588/speeches',
	'/api/meeting/termin-10574',
	'/api/meeting/termin-10588',
	'/api/speeches/analytics?meeting=termin-1',
	'/api/meeting/termin-1/speeches',
];
// summation order, libm noise
const eps = 1e-9;
// go adds member attribution
const goOnly = /\.votes\.\d+\.members\.\d+\.(party_code|person_id)$/;

function diff(a, b, path, out) {
	if (typeof a === 'number' && typeof b === 'number') {
		if (Math.abs(a - b) > eps * Math.max(1, Math.abs(a)))
			out.push(`${path}: ${a} != ${b}`);
		return;
	}
	if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object') {
		if (a !== b) out.push(`${path}: ${JSON.stringify(a)} != ${JSON.stringify(b)}`);
		return;
	}
	if (Array.isArray(a) !== Array.isArray(b)) {
		out.push(`${path}: array vs object`);
		return;
	}
	if (Array.isArray(a) && a.length !== b.length)
		out.push(`${path}: length ${a.length} != ${b.length}`);
	const keys = new Set([...Object.keys(a), ...Object.keys(b)]);
	for (const k of keys) {
		if (!(k in b) && goOnly.test(`${path}.${k}`)) continue;
		if (!(k in a)) out.push(`${path}.${k}: missing in go`);
		else if (!(k in b)) out.push(`${path}.${k}: missing in python`);
		else diff(a[k], b[k], `${path}.${k}`, out);
	}
}

let failed = false;
for (const p of paths) {
	const [g, py] = await Promise.all(
		[goBase, pyBase].map((b) => fetch(b + p).then((r) => r.json())),
	);
	const out = [];
	diff(g, py, '$', out);
	console.log(`${out.length ? 'DIFF' : 'SAME'} ${p}`);
	for (const line of out.slice(0, 40)) console.log('  ' + line);
	if (out.length > 40) console.log(`  ... ${out.length - 40} more`);
	failed ||= out.length > 0;
}
console.log(failed ? 'FAILURES' : 'ALL PASS');
process.exitCode = failed ? 1 : 0;
