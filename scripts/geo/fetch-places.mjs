// Exports named features inside Stadtkreis Karlsruhe from the Overpass API
// to geo/places-ka.json, one point per (name, kind).
//
//	node scripts/geo/fetch-places.mjs [--raw saved-overpass.json]
//
// --raw reuses a saved Overpass response instead of querying again.
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, '..', '..', 'geo', 'places-ka.json');

// overpass returns whole geometries of features that cross the city
// boundary (the Rhine relation spans four countries); points outside this
// box around the Stadtkreis are dropped before a centre is computed
const box = { s: 48.93, w: 8.27, n: 49.1, e: 8.55 };
const inBox = (p) => p && p.lat >= box.s && p.lat <= box.n && p.lon >= box.w && p.lon <= box.e;

const re = (s) => new RegExp(`^(${s})$`);
const kindOf = (t) => {
	if ((t.boundary === 'administrative' && re('9|10').test(t.admin_level)) ||
		re('suburb|neighbourhood|quarter').test(t.place))
		return 'district';
	if (re('station|halt|tram_stop|funicular').test(t.railway) || t.public_transport === 'station')
		return 'transit';
	if (t.highway) {
		if (re('platform|bus_stop|elevator').test(t.highway)) return null;
		return /platz$/i.test(t.name) || t.place === 'square' ? 'square' : 'street';
	}
	if (t.place === 'square' || t.amenity === 'marketplace') return 'square';
	if (re('park|garden|common').test(t.leisure) ||
		re('cemetery|grass|meadow|village_green').test(t.landuse))
		return 'park';
	if (re('school|kindergarten|college|university|music_school').test(t.amenity)) return 'school';
	if (t.waterway || t.water || re('water|wetland').test(t.natural)) return 'water';
	if (t.landuse || t.natural === 'wood' || t.leisure === 'nature_reserve' || t.place === 'locality')
		return 'area';
	if (t.amenity || t.building || t.tourism || t.historic || t.leisure || t.man_made || t.power || t.shop)
		return 'building';
	return null;
};

const dist = (a, b) => Math.hypot(a.lat - b.lat, (a.lon - b.lon) * Math.cos((a.lat * Math.PI) / 180));
const length = (pts) => pts.slice(1).reduce((s, p, i) => s + dist(pts[i], p), 0);

const midpoint = (pts) => {
	let half = length(pts) / 2;
	for (let i = 1; i < pts.length; i++) {
		const d = dist(pts[i - 1], pts[i]);
		if (d >= half && d > 0) {
			const f = half / d;
			return {
				lat: pts[i - 1].lat + f * (pts[i].lat - pts[i - 1].lat),
				lon: pts[i - 1].lon + f * (pts[i].lon - pts[i - 1].lon),
			};
		}
		half -= d;
	}
	return pts[0];
};

// shoelace centroid; area is in squared degrees and only compared
const polygon = (pts) => {
	let a = 0, x = 0, y = 0;
	for (let i = 0; i < pts.length - 1; i++) {
		const c = pts[i].lon * pts[i + 1].lat - pts[i + 1].lon * pts[i].lat;
		a += c;
		x += (pts[i].lon + pts[i + 1].lon) * c;
		y += (pts[i].lat + pts[i + 1].lat) * c;
	}
	if (Math.abs(a) < 1e-12) return { area: 0, pt: mean(pts) };
	return { area: Math.abs(a / 2), pt: { lat: y / (3 * a), lon: x / (3 * a) } };
};

const mean = (pts) => ({
	lat: pts.reduce((s, p) => s + p.lat, 0) / pts.length,
	lon: pts.reduce((s, p) => s + p.lon, 0) / pts.length,
});

// rank orders candidates for one (name, kind): relation > area > line > node
const locate = (el) => {
	if (el.type === 'node') return inBox(el) ? { rank: 0, size: 0, pt: el } : null;
	if (el.type === 'way') {
		const g = el.geometry || [];
		const closed = g.length > 3 && g[0].lat === g.at(-1).lat && g[0].lon === g.at(-1).lon;
		if (closed && (!el.tags.highway || el.tags.area === 'yes')) {
			const p = polygon(g);
			return inBox(p.pt) ? { rank: 2, size: p.area, pt: p.pt } : null;
		}
		const pts = g.filter(inBox);
		return pts.length ? { rank: 1, size: length(pts), pt: midpoint(pts) } : null;
	}
	const ms = (el.members || []).filter((m) => m.geometry);
	const outer = ms.filter((m) => m.role === 'outer');
	const pts = (outer.length ? outer : ms).flatMap((m) => m.geometry).filter(inBox);
	return pts.length ? { rank: 3, size: pts.length, pt: mean(pts) } : null;
};

const altNames = (t) =>
	['alt_name', 'short_name', 'official_name', 'loc_name']
		.flatMap((k) => (t[k] ? t[k].split(';') : []))
		.map((s) => s.trim())
		.filter((s) => s && s !== t.name);

const rawIdx = process.argv.indexOf('--raw');
let data;
if (rawIdx > 0) {
	data = JSON.parse(readFileSync(process.argv[rawIdx + 1], 'utf8'));
} else {
	const query = readFileSync(join(here, 'places-ka.overpassql'), 'utf8');
	const res = await fetch('https://overpass-api.de/api/interpreter', {
		method: 'POST',
		headers: { 'User-Agent': 'desinformationssystem gazetteer export' },
		body: new URLSearchParams({ data: query }),
	});
	if (!res.ok) throw new Error(`overpass: ${res.status} ${await res.text()}`);
	data = await res.json();
}

const best = new Map();
for (const el of data.elements) {
	const t = el.tags || {};
	const kind = kindOf(t);
	if (!kind) continue;
	const loc = locate(el);
	if (!loc) continue;
	const key = `${t.name}\u0000${kind}`;
	const cur = best.get(key);
	const alt = new Set([...(cur?.alt || []), ...altNames(t)]);
	if (!cur || loc.rank > cur.rank || (loc.rank === cur.rank && loc.size > cur.size)) {
		best.set(key, { name: t.name, kind, ...loc, osm: `${el.type}/${el.id}`, alt });
	} else {
		cur.alt = alt;
	}
}

const round = (x) => Math.round(x * 1e5) / 1e5;
const rows = [...best.values()]
	.map((p) => {
		const r = { name: p.name, kind: p.kind, lat: round(p.pt.lat), lon: round(p.pt.lon), osm: p.osm };
		const alt = [...p.alt].filter((a) => a !== p.name).sort();
		if (alt.length) r.alt = alt;
		return r;
	})
	.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : a.kind < b.kind ? -1 : 1));

writeFileSync(out, '[\n' + rows.map((r) => JSON.stringify(r)).join(',\n') + '\n]\n');
console.log(`${rows.length} places -> ${out}`);
