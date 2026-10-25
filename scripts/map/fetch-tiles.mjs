// needs go-pmtiles ($PMTILES)
import { execFileSync } from 'node:child_process';
import { mkdirSync, renameSync } from 'node:fs';
import { dirname } from 'node:path';

const out = process.argv[2] ?? 'data/map/karlsruhe.pmtiles';
const pmtiles = process.env.PMTILES ?? 'pmtiles';
// stadtteile bbox + 2 km
const BBOX = '8.25,48.92,8.57,49.11';

const r = await fetch('https://build-metadata.protomaps.dev/builds.json');
if (!r.ok) throw new Error(`builds.json: ${r.status}`);
const keys = (await r.json()).map((b) => b.key).sort();
const url = `https://build.protomaps.com/${keys.at(-1)}`;
console.log(`[fetch-tiles] ${url} -> ${out}`);

mkdirSync(dirname(out), { recursive: true });
const tmp = `${out}.part`;
execFileSync(
	pmtiles,
	['extract', url, tmp, `--bbox=${BBOX}`, '--maxzoom=15'],
	{ stdio: 'inherit' },
);
renameSync(tmp, out);
