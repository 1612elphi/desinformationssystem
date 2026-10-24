import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import stadtteile from 'ui/stadtteile.json';

type Ring = number[][];
interface Feature {
  name: string;
  type: string;
  coordinates: Ring[] | Ring[][];
}

// Choropleth of the 27 Stadtteile shaded by document count; a click filters
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
  const outer = rings(f)[0] ?? [];
  const mean = (i: number) =>
    outer.reduce((a, p) => a + (p[i] ?? 0), 0) / (outer.length || 1);
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
    cx: px(mean(0)),
    cy: py(mean(1)),
  };
});
const VIEWBOX = `0 0 ${W} ${H.toFixed(0)}`;

interface Signature {
  Args: { model: ApplicationModel };
}

export default class KarteTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get counts() {
    return this.args.model.facets.value?.district_counts ?? {};
  }

  get max() {
    return Math.max(1, ...Object.values(this.counts));
  }

  count = (name: string) => this.counts[name] || 0;

  fill = (name: string) => {
    const n = this.count(name);
    return n
      ? `rgba(0,157,154,${(0.18 + 0.72 * Math.sqrt(n / this.max)).toFixed(2)})`
      : 'rgba(141,141,141,0.22)';
  };

  select = (name: string) => this.nav.filter({ district: name });

  <template>
    <div class="dis-panel dis-map">
      <svg
        viewBox={{VIEWBOX}}
        class="dis-map-svg"
        role="img"
        aria-label="Karte der Stadtteile nach Dokumentanzahl"
      >
        {{#each SHAPES as |s|}}
          <path
            d={{s.d}}
            fill={{this.fill s.name}}
            class="dis-teil"
            {{on "click" (fn this.select s.name)}}
          >
            <title>{{s.name}}: {{this.count s.name}}</title>
          </path>
        {{/each}}
        {{#each SHAPES as |s|}}
          <text
            x={{s.cx}}
            y={{s.cy}}
            class="dis-teil-lbl"
            {{on "click" (fn this.select s.name)}}
          >{{s.name}}</text>
        {{/each}}
      </svg>
    </div>
  </template>
}
