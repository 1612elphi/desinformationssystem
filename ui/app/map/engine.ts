import {
  AttributionControl,
  Map as MapLibre,
  addProtocol,
  setWorkerUrl,
  type ExpressionSpecification,
  type GeoJSONSourceSpecification,
  type LayerSpecification,
  type StyleSpecification,
} from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url';
import { Protocol } from 'pmtiles';
import { layers, namedFlavor } from '@protomaps/basemaps';
import stadtteile from 'ui/stadtteile.json';
import { KINDS, type Kind, type Place } from 'ui/utils/api-karte';

export interface Colours {
  dark: boolean;
  [key: string]: string | boolean;
}

export interface MapState {
  colours: Colours;
  places: Place[];
  kinds: Kind[];
  districts: boolean;
  counts: Record<string, number>;
  selected: number;
}

export interface MapEvents {
  onPick: (id: number) => void;
  onDistrict: (name: string) => void;
}

setWorkerUrl(workerUrl);
addProtocol('pmtiles', new Protocol().tile);

// tile extract bounds, see scripts/map/fetch-tiles.mjs
const BOUNDS: [number, number, number, number] = [8.25, 48.92, 8.57, 49.11];
const CITY: [number, number, number, number] = [8.277, 48.94, 8.542, 49.092];
// heatmap gives way to blobs between these zooms
const Z_FADE = [12.5, 14] as const;
const STEPS = 4;

type Ring = number[][];
interface Teil {
  name: string;
  type: string;
  coordinates: Ring[] | Ring[][];
}
const TEILE = (stadtteile as { features: Teil[] }).features;

const c = (s: MapState, key: string) => String(s.colours[key] ?? '#888');

function flavor(s: MapState) {
  const base = namedFlavor(s.colours.dark ? 'black' : 'white');
  const roads = {
    other: c(s, 'road'),
    minor_service: c(s, 'road'),
    minor_a: c(s, 'road'),
    minor_b: c(s, 'road'),
    link: c(s, 'road'),
    major: c(s, 'major'),
    highway: c(s, 'highway'),
    bridges_other: c(s, 'road'),
    bridges_minor: c(s, 'road'),
    bridges_link: c(s, 'road'),
    bridges_major: c(s, 'major'),
    bridges_highway: c(s, 'highway'),
    tunnel_other: c(s, 'road'),
    tunnel_minor: c(s, 'road'),
    tunnel_link: c(s, 'road'),
    tunnel_major: c(s, 'major'),
    tunnel_highway: c(s, 'highway'),
  };
  return {
    ...base,
    ...roads,
    background: c(s, 'bg'),
    earth: c(s, 'earth'),
    water: c(s, 'water'),
    park_a: c(s, 'park'),
    park_b: c(s, 'park'),
    wood_a: c(s, 'park'),
    wood_b: c(s, 'park'),
    scrub_a: c(s, 'park'),
    scrub_b: c(s, 'park'),
    buildings: c(s, 'building'),
    railway: c(s, 'rail'),
    boundaries: c(s, 'casing'),
    roads_label_minor: c(s, 'label'),
    roads_label_minor_halo: c(s, 'bg'),
    roads_label_major: c(s, 'label'),
    roads_label_major_halo: c(s, 'bg'),
    subplace_label: c(s, 'label'),
    subplace_label_halo: c(s, 'bg'),
    city_label: c(s, 'fg'),
    city_label_halo: c(s, 'bg'),
    // no basemap POI layer; its icons compete with the place blobs
    pois: undefined,
  };
}

const placesSource = (s: MapState): GeoJSONSourceSpecification => ({
  type: 'geojson',
  data: {
    type: 'FeatureCollection',
    features: s.places.map((p) => ({
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [p.lon, p.lat] },
      properties: {
        id: p.id,
        name: p.name,
        kind: p.kind,
        docs: p.docs,
        mentions: p.mentions,
      },
    })),
  },
});

const teileSource = (s: MapState): GeoJSONSourceSpecification => ({
  type: 'geojson',
  data: {
    type: 'FeatureCollection',
    features: TEILE.map((t) => ({
      type: 'Feature',
      geometry: { type: t.type, coordinates: t.coordinates },
      properties: { name: t.name, n: s.counts[t.name] ?? 0 },
    })),
  },
});

const kindColour = (s: MapState): ExpressionSpecification =>
  [
    'match',
    ['get', 'kind'],
    ...KINDS.flatMap((k) => [k, c(s, `kind-${k}`)]),
    c(s, 'fg'),
  ] as unknown as ExpressionSpecification;

const visible = (on: boolean) =>
  ({ visibility: on ? 'visible' : 'none' }) as const;

function dataLayers(s: MapState): LayerSpecification[] {
  const max = Math.max(1, ...s.places.map((p) => p.mentions));
  const maxN = Math.max(1, ...Object.values(s.counts));
  const kindFilter: ExpressionSpecification = [
    'in',
    ['get', 'kind'],
    ['literal', s.kinds],
  ];
  // sqrt so a place with four times the documents has twice the radius
  const r: ExpressionSpecification = [
    '+',
    3,
    ['*', 2.5, ['sqrt', ['get', 'docs']]],
  ];
  const fade = (from: number, to: number): ExpressionSpecification => [
    'interpolate',
    ['linear'],
    ['zoom'],
    Z_FADE[0],
    from,
    Z_FADE[1],
    to,
  ];
  const step: ExpressionSpecification = [
    'step',
    ['sqrt', ['/', ['get', 'n'], maxN]],
    c(s, 'step-0'),
    0.0001,
    c(s, 'step-1'),
    ...Array.from({ length: STEPS - 1 }, (_, i) => [
      (i + 1) / STEPS,
      c(s, `step-${i + 2}`),
    ]).flat(),
  ] as ExpressionSpecification;
  return [
    {
      id: 'teile-fill',
      type: 'fill',
      source: 'teile',
      layout: visible(s.districts),
      paint: { 'fill-color': step, 'fill-opacity': 0.45 },
    },
    {
      id: 'teile-line',
      type: 'line',
      source: 'teile',
      layout: visible(s.districts),
      paint: { 'line-color': c(s, 'fg'), 'line-width': 1 },
    },
    {
      id: 'places-heat',
      type: 'heatmap',
      source: 'places',
      maxzoom: Z_FADE[1] + 0.5,
      filter: kindFilter,
      paint: {
        'heatmap-weight': [
          '/',
          ['ln', ['+', 1, ['get', 'mentions']]],
          Math.log(1 + max),
        ],
        'heatmap-intensity': [
          'interpolate',
          ['linear'],
          ['zoom'],
          10,
          0.5,
          14,
          1.2,
        ],
        'heatmap-radius': ['interpolate', ['linear'], ['zoom'], 10, 10, 14, 36],
        'heatmap-color': [
          'interpolate',
          ['linear'],
          ['heatmap-density'],
          0,
          c(s, 'heat-0'),
          0.15,
          c(s, 'heat-1'),
          0.5,
          c(s, 'heat-2'),
          1,
          c(s, 'heat-3'),
        ],
        'heatmap-opacity': fade(0.85, 0),
      },
    },
    {
      id: 'places-dot',
      type: 'circle',
      source: 'places',
      minzoom: Z_FADE[0],
      filter: kindFilter,
      layout: { 'circle-sort-key': ['get', 'docs'] },
      paint: {
        'circle-radius': [
          'interpolate',
          ['linear'],
          ['zoom'],
          Z_FADE[0],
          ['*', 0.7, r],
          17,
          ['*', 1.8, r],
        ],
        'circle-color': kindColour(s),
        'circle-opacity': fade(0, 0.85),
        'circle-stroke-color': c(s, 'bg'),
        'circle-stroke-width': 1,
        'circle-stroke-opacity': fade(0, 1),
      },
    },
    {
      id: 'places-selected',
      type: 'circle',
      source: 'places',
      filter: ['==', ['get', 'id'], s.selected],
      paint: {
        'circle-radius': [
          'interpolate',
          ['linear'],
          ['zoom'],
          10,
          8,
          17,
          ['*', 1.8, ['+', 6, r]],
        ],
        'circle-color': 'rgba(0,0,0,0)',
        'circle-stroke-color': c(s, 'ring'),
        'circle-stroke-width': 3,
      },
    },
    {
      id: 'places-label',
      type: 'symbol',
      source: 'places',
      minzoom: 14.5,
      filter: kindFilter,
      layout: {
        'text-field': ['get', 'name'],
        'text-font': ['Noto Sans Medium'],
        'text-size': 12,
        'text-anchor': 'top',
        'text-offset': [0, 0.9],
        'text-optional': true,
        'symbol-sort-key': ['-', ['get', 'docs']],
      },
      paint: {
        'text-color': c(s, 'fg'),
        'text-halo-color': c(s, 'bg'),
        'text-halo-width': 1.5,
      },
    },
    {
      id: 'teile-label',
      type: 'symbol',
      source: 'teile',
      maxzoom: 14,
      layout: {
        ...visible(s.districts),
        'text-field': ['get', 'name'],
        'text-font': ['Noto Sans Medium'],
        'text-size': 12,
      },
      paint: {
        'text-color': c(s, 'fg'),
        'text-halo-color': c(s, 'bg'),
        'text-halo-width': 1.5,
      },
    },
  ];
}

function style(s: MapState): StyleSpecification {
  const origin = location.origin;
  return {
    version: 8,
    glyphs: `${origin}/map/fonts/{fontstack}/{range}.pbf`,
    sprite: `${origin}/map/sprites/${s.colours.dark ? 'dark' : 'light'}`,
    sources: {
      protomaps: {
        type: 'vector',
        url: `pmtiles://${origin}/map/karlsruhe.pmtiles`,
        attribution:
          '<a href="https://www.openstreetmap.org/copyright">© OpenStreetMap</a>',
      },
      teile: teileSource(s),
      places: placesSource(s),
    },
    layers: [
      ...layers('protomaps', flavor(s), { lang: 'de' }),
      ...dataLayers(s),
    ],
  };
}

export default class MapEngine {
  map: MapLibre;
  state: MapState;
  events: MapEvents;

  constructor(el: HTMLElement, state: MapState, events: MapEvents) {
    this.state = state;
    this.events = events;
    this.map = new MapLibre({
      container: el,
      style: style(state),
      bounds: CITY,
      maxBounds: BOUNDS,
      minZoom: 10,
      maxZoom: 18,
      dragRotate: false,
      pitchWithRotate: false,
      attributionControl: false,
      locale: {
        'Map.Title': 'Karte',
        'AttributionControl.ToggleAttribution': 'Quellen',
      },
    });
    this.map.touchZoomRotate.disableRotation();
    this.map.addControl(
      new AttributionControl({
        compact: false,
        customAttribution: '<a href="https://protomaps.com">Protomaps</a>',
      }),
      'bottom-left',
    );
    this.map.on('click', 'places-dot', (e) => {
      const id = e.features?.[0]?.properties['id'] as number | undefined;
      if (id) this.events.onPick(id);
    });
    this.map.on('click', (e) => {
      if (!this.state.districts) return;
      const hit = this.map.queryRenderedFeatures(e.point, {
        layers: ['places-dot', 'teile-fill'],
      });
      if (hit[0]?.layer.id === 'teile-fill')
        this.events.onDistrict(hit[0].properties['name'] as string);
    });
    for (const layer of ['places-dot', 'teile-fill']) {
      this.map.on('mouseenter', layer, () => {
        this.map.getCanvas().style.cursor = 'pointer';
      });
      this.map.on('mouseleave', layer, () => {
        this.map.getCanvas().style.cursor = '';
      });
    }
    this.map.getCanvas().addEventListener('keydown', this.enter);
  }

  update(state: MapState) {
    const old = this.state;
    const keys = Object.keys(state) as (keyof MapState)[];
    if (keys.every((k) => state[k] === old[k])) return;
    this.state = state;
    this.map.setStyle(style(state), { diff: true });
  }

  // Enter on the focused map picks the place nearest the centre
  enter = (e: KeyboardEvent) => {
    if (e.key !== 'Enter') return;
    const mid = this.map.project(this.map.getCenter());
    let best: Place | undefined;
    let dist = 48;
    for (const p of this.state.places) {
      if (!this.state.kinds.includes(p.kind)) continue;
      const q = this.map.project([p.lon, p.lat]);
      const d = Math.hypot(q.x - mid.x, q.y - mid.y);
      if (d < dist) [best, dist] = [p, d];
    }
    if (best) this.events.onPick(best.id);
  };

  flyTo(p: Place, right: number) {
    this.map.flyTo({
      center: [p.lon, p.lat],
      zoom: Math.max(this.map.getZoom(), 15),
      padding: { top: 0, bottom: 0, left: 0, right },
    });
  }

  destroy() {
    this.map.getCanvas().removeEventListener('keydown', this.enter);
    this.map.remove();
  }
}
