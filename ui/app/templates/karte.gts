import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import type RouterService from '@ember/routing/router-service';
import { pageTitle } from 'ember-page-title';
import CanonPage from 'delphicanon/components/canon-page';
import DataTable from 'ui/components/data-table';
import { InlineLoading, LoadError } from 'ui/components/loading';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import stadtteile from 'ui/stadtteile.json';
import { eq, num } from 'ui/utils/meta';
import './karte.css';

type Ring = number[][];
interface Feature {
  name: string;
  type: string;
  coordinates: Ring[] | Ring[][];
}

// Choropleth of the 27 Stadtteile shaded by document count; a district filters
// the Dokumente list. Boundaries: app/stadtteile.json (simplified), inline SVG.
const FEATURES = (stadtteile as { features: Feature[] }).features;
const rings = (f: Feature): Ring[] =>
  f.type === 'MultiPolygon'
    ? (f.coordinates as Ring[][]).flat()
    : (f.coordinates as Ring[]);

let minX = Infinity,
  minY = Infinity,
  maxX = -Infinity,
  maxY = -Infinity;
for (const f of FEATURES)
  for (const r of rings(f))
    for (const [x = 0, y = 0] of r) {
      minX = Math.min(minX, x);
      maxX = Math.max(maxX, x);
      minY = Math.min(minY, y);
      maxY = Math.max(maxY, y);
    }
// longitude degrees are shorter this far north
const kx = Math.cos((((minY + maxY) / 2) * Math.PI) / 180);
const W = 780;
const H = (W * (maxY - minY)) / ((maxX - minX) * kx);
const px = (x: number) => ((x - minX) / (maxX - minX)) * W;
const py = (y: number) => ((maxY - y) / (maxY - minY)) * H;

const SHAPES = FEATURES.map((f) => {
  const outer = (rings(f)[0] ?? []).map(([x = 0, y = 0]) => [px(x), py(y)]);
  const mean = (i: number) =>
    outer.reduce((a, p) => a + (p[i] ?? 0), 0) / (outer.length || 1);
  const area = Math.abs(
    outer.reduce((a, [x = 0, y = 0], i) => {
      const [nx = 0, ny = 0] = outer[(i + 1) % outer.length] ?? [];
      return a + x * ny - nx * y;
    }, 0) / 2,
  );
  return {
    name: f.name,
    d: rings(f)
      .map(
        (r) =>
          'M' +
          r
            .map(([x = 0, y = 0]) => `${px(x).toFixed(1)} ${py(y).toFixed(1)}`)
            .join('L') +
          'Z',
      )
      .join(' '),
    cx: mean(0),
    cy: mean(1),
    area,
  };
});

// in viewBox units; karte.css hides labels below the width where this is legible
const LABEL_SIZE = 12;
// ponytail: width estimated from character count, not measured text
const box = (s: { name: string; cx: number; cy: number }) => {
  const w = s.name.length * LABEL_SIZE * 0.55 + 4;
  const h = LABEL_SIZE * 1.3;
  return {
    x0: s.cx - w / 2,
    x1: s.cx + w / 2,
    y0: s.cy - h / 2,
    y1: s.cy + h / 2,
  };
};
const placed: ReturnType<typeof box>[] = [];
// larger districts claim their label first; a label that would overlap one is dropped
const LABELS = [...SHAPES]
  .sort((a, b) => b.area - a.area)
  .filter((s) => {
    const b = box(s);
    if (
      placed.some(
        (p) => b.x0 < p.x1 && p.x0 < b.x1 && b.y0 < p.y1 && p.y0 < b.y1,
      )
    )
      return false;
    placed.push(b);
    return true;
  });

const VIEWBOX = `0 0 ${W} ${H.toFixed(0)}`;
const STEPS = 4;
const COLUMNS = [
  { key: 'name', label: 'Stadtteil' },
  { key: 'count', label: 'Dokumente', width: '8rem', cls: 'dis-num' },
];

// ∑CG: error row when the district document counts did not load
//   spec: one short sentence, plain German, says the counts per district are unavailable; shown after "Fehler: "
//   sample: "Dokumentanzahlen je Stadtteil sind nicht verfügbar."
const COUNTS_MISSING = '∑CG';

interface Signature {
  Args: { model: ApplicationModel };
}

export default class KarteTemplate extends Component<Signature> {
  @service declare nav: NavService;
  @service declare router: RouterService;

  get facets() {
    return this.args.model.facets;
  }

  get error() {
    if (this.facets.error) return this.facets.error;
    if (this.facets.value && !this.facets.value.district_counts)
      return COUNTS_MISSING;
    return '';
  }

  get counts() {
    return this.facets.value?.district_counts;
  }

  get max() {
    return Math.max(1, ...Object.values(this.counts ?? {}));
  }

  count = (name: string) => this.counts?.[name] || 0;

  // upper bound of a step; square-root scale, so small counts stay apart
  bound = (k: number) => Math.floor((this.max * k * k) / (STEPS * STEPS));

  step = (name: string) => {
    const n = this.count(name);
    if (!n) return 0;
    let k = 1;
    while (k < STEPS && n > this.bound(k)) k++;
    return k;
  };

  get legend() {
    const items = [{ step: 0, text: '0' }];
    for (let k = 1; k <= STEPS; k++) {
      const lo = this.bound(k - 1) + 1;
      const hi = k === STEPS ? this.max : this.bound(k);
      if (lo <= hi)
        items.push({
          step: k,
          text: lo === hi ? num(lo) : `${num(lo)}–${num(hi)}`,
        });
    }
    return items;
  }

  get rows() {
    return SHAPES.map((s) => ({
      name: s.name,
      count: this.count(s.name),
    })).sort((a, b) => a.name.localeCompare(b.name, 'de'));
  }

  label = (name: string) => `${name}: ${num(this.count(name))} Dokumente`;
  rowLabel = (row: { name: string }) => this.label(row.name);

  href = (name: string) =>
    this.router.urlFor('index', { queryParams: { district: name } });

  filter = (name: string) => this.nav.filter({ district: name });
  openRow = (row: { name: string }) => this.filter(row.name);

  // modified clicks keep the browser's own link handling (new tab, window)
  click = (name: string, event: MouseEvent) => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    this.filter(name);
  };

  <template>
    {{pageTitle "Karte"}}
    <CanonPage class="dis-karte" @maxWidth={{1080}}>
      <h1 class="dis-visually-hidden">Karte</h1>
      {{#if this.error}}
        <LoadError @error={{this.error}} />
      {{else if this.counts}}
        <div class="dis-map">
          <svg
            viewBox={{VIEWBOX}}
            class="dis-map-svg"
            role="group"
            aria-label="Karte der Stadtteile nach Dokumentanzahl"
          >
            {{#each SHAPES as |s|}}
              <a
                href={{this.href s.name}}
                class="dis-teil dis-step-{{this.step s.name}}"
                aria-label={{this.label s.name}}
                {{on "click" (fn this.click s.name)}}
              ><path d={{s.d}} /></a>
            {{/each}}
            {{#each LABELS as |s|}}
              <text
                x={{s.cx}}
                y={{s.cy}}
                class="dis-teil-lbl"
                aria-hidden="true"
              >{{s.name}}</text>
            {{/each}}
          </svg>
          <div class="dis-legend" aria-hidden="true">
            <span class="dis-legend-title">Dokumente</span>
            {{#each this.legend as |l|}}
              <span class="dis-legend-item">
                <span class="dis-swatch dis-step-{{l.step}}"></span>
                {{l.text}}
              </span>
            {{/each}}
          </div>
        </div>
        <DataTable
          @columns={{COLUMNS}}
          @rows={{this.rows}}
          @caption="Stadtteile"
          @onOpen={{this.openRow}}
          @rowLabel={{this.rowLabel}}
          as |row col|
        >
          {{#if (eq col.key "name")}}
            {{row.name}}
          {{else}}
            {{num row.count}}
          {{/if}}
        </DataTable>
      {{else}}
        <InlineLoading @text="Lädt…" />
      {{/if}}
    </CanonPage>
  </template>
}
