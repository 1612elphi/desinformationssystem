import Component from '@glimmer/component';
import { cached, tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import { htmlSafe } from '@ember/template';
import type RouterService from '@ember/routing/router-service';
import { pageTitle } from 'ember-page-title';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonPage from 'delphicanon/components/canon-page';
import CanonRows from 'delphicanon/components/canon-rows';
import { search, type SearchResult } from 'ui/api';
import Icon from 'ui/components/icon';
import { InlineLoading, LoadError, LoadState } from 'ui/components/loading';
import Tag from 'ui/components/tag';
import rough from 'ui/modifiers/rough';
import shortcut from 'ui/modifiers/shortcut';
import Load from 'ui/utils/load';
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
const LATEST = 5;
const dateDe = (iso?: string) =>
  iso ? iso.split('-').reverse().join('.') : '';
const first = (i: number) => i === 0;
const rowId = (name: string) => `dis-district-${name.replace(/\W+/g, '-')}`;

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

  // the district under the pointer or focus, on the map or in the list
  @tracked hot = '';
  @tracked selected = '';

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

  // ranked by count, so the list reads as a bar chart
  get rows() {
    return SHAPES.map((s) => ({
      name: s.name,
      id: rowId(s.name),
      count: this.count(s.name),
      bar: htmlSafe(`inline-size: ${(this.count(s.name) / this.max) * 100}%`),
    })).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name, 'de'));
  }

  label = (name: string) => `${name}: ${num(this.count(name))} Dokumente`;

  href = (name: string) =>
    this.router.urlFor('index', { queryParams: { district: name } });

  stateOf = (name: string) =>
    [
      name === this.hot ? 'dis-teil--hot' : '',
      name === this.selected ? 'dis-teil--selected' : '',
    ].join(' ');

  enter = (name: string) => (this.hot = name);
  leave = () => (this.hot = '');

  // from the map the list row also scrolls into view inside its pane
  enterMap = (name: string) => {
    this.hot = name;
    document.getElementById(rowId(name))?.scrollIntoView({ block: 'nearest' });
  };

  select = (name: string) =>
    (this.selected = this.selected === name ? '' : name);
  unselect = () => (this.selected = '');

  #lastLatest?: Load<SearchResult>;

  @cached
  get latest() {
    if (!this.selected) return undefined;
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#lastLatest = new Load(
      search({ district: this.selected, limit: LATEST }),
      this.#lastLatest,
    ));
  }

  filter = () => this.nav.filter({ district: this.selected });
  openDoc = (id: string) => this.nav.open({ doc: id });

  // modified clicks keep the browser's own link handling (new tab, window)
  click = (name: string, event: MouseEvent) => {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
      return;
    event.preventDefault();
    this.select(name);
  };

  <template>
    {{pageTitle "Karte"}}
    <CanonPage class="dis-karte" @maxWidth={{1280}}>
      <h1 class="dis-visually-hidden">Karte</h1>
      {{#if this.error}}
        <LoadError @error={{this.error}} />
      {{else if this.counts}}
        <div class="dis-karte-body">
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
                  class="dis-teil dis-step-{{this.step s.name}}
                    {{this.stateOf s.name}}"
                  aria-label={{this.label s.name}}
                  {{on "click" (fn this.click s.name)}}
                  {{on "mouseenter" (fn this.enterMap s.name)}}
                  {{on "mouseleave" this.leave}}
                  {{on "focus" (fn this.enterMap s.name)}}
                  {{on "blur" this.leave}}
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
              <span class="dis-legend-title"><Icon
                  @name="docs-files"
                  @size={{14}}
                />Dokumente</span>
              {{#each this.legend as |l|}}
                <span class="dis-legend-item">
                  <span class="dis-swatch dis-step-{{l.step}}"></span>
                  {{l.text}}
                </span>
              {{/each}}
            </div>

            {{#if this.selected}}
              <section
                class="dis-district-panel"
                aria-label={{this.selected}}
                {{shortcut "Escape" this.unselect}}
              >
                <CanonRows @framed={{true}}>
                  <div class="dis-district-head">
                    <Icon @name="docs-district" @size={{20}} />
                    <h2
                      class="dis-district-title"
                      {{rough
                        "underline"
                        color="--dis-highlight"
                        padding=2
                        strokeWidth=2
                      }}
                    >{{this.selected}}</h2>
                    <span class="dis-district-count">{{num
                        (this.count this.selected)
                      }}
                      Dokumente</span>
                    <CanonButton
                      @kind="cell"
                      @icon={{component Icon name="close"}}
                      @label="Schließen"
                      @onClick={{this.unselect}}
                    />
                  </div>
                  <LoadState @load={{this.latest}}>
                    {{#each this.latest.value.results key="id" as |d|}}
                      <button
                        type="button"
                        class="dis-latest"
                        {{on "click" (fn this.openDoc d.id)}}
                      >
                        <span class="dis-latest-date">{{dateDe
                            d.meeting_date
                          }}</span>
                        <span class="dis-latest-label">{{d.label}}</span>
                        {{#if d.doc_type}}<Tag>{{d.doc_type}}</Tag>{{/if}}
                      </button>
                    {{else}}
                      <CanonEmptyRow @text="Keine Dokumente" />
                    {{/each}}
                  </LoadState>
                  <CanonButton
                    @kind="primary"
                    @text="Alle Dokumente"
                    @icon={{component Icon name="chevron-right"}}
                    @onClick={{this.filter}}
                  />
                </CanonRows>
              </section>
            {{/if}}
          </div>

          <ul class="dis-districts" aria-label="Stadtteile">
            {{#each this.rows key="name" as |row i|}}
              <li>
                <button
                  type="button"
                  id={{row.id}}
                  class="dis-district {{this.stateOf row.name}}"
                  aria-label={{this.label row.name}}
                  aria-pressed={{if (eq this.selected row.name) "true" "false"}}
                  {{on "click" (fn this.select row.name)}}
                  {{on "mouseenter" (fn this.enter row.name)}}
                  {{on "mouseleave" this.leave}}
                  {{on "focus" (fn this.enter row.name)}}
                  {{on "blur" this.leave}}
                >
                  <span class="dis-district-name">{{row.name}}</span>
                  <span class="dis-district-track" aria-hidden="true"><span
                      class="dis-district-bar dis-step-{{this.step row.name}}"
                      style={{row.bar}}
                    ></span></span>
                  <span class="dis-district-n"><span
                      {{rough
                        "circle"
                        color="--dis-yes"
                        padding=4
                        show=(first i)
                        delay=600
                      }}
                    >{{num row.count}}</span></span>
                </button>
              </li>
            {{/each}}
          </ul>
        </div>
      {{else}}
        <InlineLoading @text="Lädt…" />
      {{/if}}
    </CanonPage>
  </template>
}
