import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { modifier } from 'ember-modifier';
import CanonButton from 'delphicanon/components/canon-button';
import Icon from 'ui/components/icon';
import { LoadError } from 'ui/components/loading';
import type { Kind, Place } from 'ui/utils/api-karte';
import type MapEngine from 'ui/map/engine';
import type { Colours, MapState } from 'ui/map/engine';
import './map-view.css';

const KEYS = [
  'bg',
  'earth',
  'water',
  'park',
  'building',
  'road',
  'major',
  'highway',
  'rail',
  'casing',
  'label',
  'fg',
  'ring',
  'heat-0',
  'heat-1',
  'heat-2',
  'heat-3',
  'step-0',
  'step-1',
  'step-2',
  'step-3',
  'step-4',
  'kind-street',
  'kind-square',
  'kind-park',
  'kind-building',
  'kind-school',
  'kind-district',
  'kind-area',
  'kind-water',
  'kind-transit',
];

// maplibre: rgb/hsl/hex only
function probe(root: HTMLElement): Colours {
  const ctx = document.createElement('canvas').getContext('2d', {
    willReadFrequently: true,
  })!;
  const out: Colours = { dark: false };
  for (const el of root.querySelectorAll<HTMLElement>('[data-k]')) {
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = getComputedStyle(el).color;
    ctx.fillRect(0, 0, 1, 1);
    const [r = 0, g = 0, b = 0, a = 0] = ctx.getImageData(0, 0, 1, 1).data;
    out[el.dataset['k']!] = `rgba(${r},${g},${b},${(a / 255).toFixed(3)})`;
    if (el.dataset['k'] === 'bg') out.dark = r + g + b < 384;
  }
  return out;
}

interface Signature {
  Args: {
    places: Place[];
    kinds: Kind[];
    districts: boolean;
    counts: Record<string, number>;
    selected: number;
    onPick: (id: number) => void;
    onDistrict: (name: string) => void;
  };
  Element: HTMLDivElement;
}

export default class MapView extends Component<Signature> {
  @tracked colours?: Colours;
  @tracked error = '';
  engine?: MapEngine;
  #flown = 0;

  get state(): MapState | undefined {
    if (!this.colours) return undefined;
    return {
      colours: this.colours,
      places: this.args.places,
      kinds: this.args.kinds,
      districts: this.args.districts,
      counts: this.args.counts,
      selected: this.args.selected,
    };
  }

  setup = modifier((el: HTMLElement) => {
    const probes = el.querySelector<HTMLElement>('.dis-map-probe')!;
    const canvas = el.querySelector<HTMLElement>('.dis-map-canvas')!;
    const retheme = () => (this.colours = probe(probes));
    const media = matchMedia('(prefers-color-scheme: dark)');
    media.addEventListener('change', retheme);
    // shell sets theme class
    const observer = new MutationObserver(retheme);
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });
    let gone = false;
    import('ui/map/engine')
      .then(({ default: Engine }) => {
        if (gone) return;
        retheme();
        if (!this.state) return;
        this.engine = new Engine(canvas, this.state, {
          onPick: (id) => this.args.onPick(id),
          onDistrict: (name) => this.args.onDistrict(name),
        });
        this.engine.map.on('error', (e) => (this.error ||= e.error.message));
      })
      .catch((e: unknown) => {
        this.error = e instanceof Error ? e.message : String(e);
      });
    return () => {
      gone = true;
      media.removeEventListener('change', retheme);
      observer.disconnect();
      this.engine?.destroy();
      this.engine = undefined;
    };
  });

  zoomIn = () => this.engine?.map.zoomIn();
  zoomOut = () => this.engine?.map.zoomOut();

  sync = modifier((el: HTMLElement, [state]: [MapState | undefined]) => {
    if (!state || !this.engine) return;
    this.engine.update(state);
    if (state.selected && state.selected !== this.#flown) {
      const p = state.places.find((x) => x.id === state.selected);
      // panel covers trailing edge
      const panel =
        el.parentElement?.querySelector<HTMLElement>('.dis-place-panel');
      const pad =
        panel && getComputedStyle(panel).position === 'absolute'
          ? panel.offsetWidth
          : 0;
      if (p) this.engine.flyTo(p, pad);
    }
    this.#flown = state.selected;
  });

  <template>
    <div
      class="dis-map-view"
      {{this.setup}}
      {{this.sync this.state}}
      ...attributes
    >
      <div class="dis-map-canvas"></div>
      <span class="dis-map-cross" aria-hidden="true"></span>
      <div class="dis-map-zoom">
        <CanonButton
          @kind="cell"
          @icon={{component Icon name="karte-plus"}}
          @label="Vergrößern"
          @onClick={{this.zoomIn}}
        />
        <CanonButton
          @kind="cell"
          @icon={{component Icon name="karte-minus"}}
          @label="Verkleinern"
          @onClick={{this.zoomOut}}
        />
      </div>
      {{#if this.error}}
        <div class="dis-map-error"><LoadError @error={{this.error}} /></div>
      {{/if}}
      <div class="dis-map-probe" aria-hidden="true">
        {{#each KEYS as |k|}}<span
            data-k={{k}}
            class="dis-{{k}}"
          ></span>{{/each}}
      </div>
    </div>
  </template>
}
