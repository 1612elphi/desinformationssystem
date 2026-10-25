import Component from '@glimmer/component';
import { cached, tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import { pageTitle } from 'ember-page-title';
import CanonRow from 'delphicanon/components/canon-row';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import CanonSwitch from 'delphicanon/components/canon-switch';
import Icon from 'ui/components/icon';
import { InlineLoading, LoadError } from 'ui/components/loading';
import MapList from 'ui/components/map-list';
import MapPanel from 'ui/components/map-panel';
import MapView from 'ui/components/map-view';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import {
  KINDS,
  KIND_LABEL,
  fetchPlaces,
  type Kind,
  type Places,
} from 'ui/utils/api-karte';
import { iso } from 'ui/utils/calendar';
import Load from 'ui/utils/load';
import { num } from 'ui/utils/meta';
import './karte.css';

const RANGES = ['Dieses Jahr', '12 Monate', 'Alles'];
const STEPS = 4;
const NO_COUNTS: Record<string, number> = {};

function from(range: number) {
  const now = new Date();
  if (range === 0) return `${now.getFullYear()}-01-01`;
  if (range === 1) {
    now.setFullYear(now.getFullYear() - 1);
    return iso(now);
  }
  return '';
}

interface Signature {
  Args: { model: ApplicationModel };
}

export default class KarteTemplate extends Component<Signature> {
  @service declare nav: NavService;

  @tracked range = 2;
  @tracked kinds: Kind[] = [...KINDS];
  @tracked districts = false;
  @tracked selected = 0;

  #last?: Load<Places>;

  @cached
  get load() {
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#last = new Load(fetchPlaces(from(this.range)), this.#last));
  }

  get all() {
    return this.load.value?.places ?? [];
  }

  @cached
  get places() {
    return this.all
      .filter((p) => this.kinds.includes(p.kind))
      .sort((a, b) => b.docs - a.docs || a.name.localeCompare(b.name, 'de'));
  }

  get place() {
    return this.all.find((p) => p.id === this.selected);
  }

  get kindItems() {
    return KINDS.map((k) => ({
      kind: k,
      label: KIND_LABEL[k],
      on: this.kinds.includes(k),
      n: this.all.filter((p) => p.kind === k).length,
    }));
  }

  get counts() {
    return this.args.model.facets.value?.district_counts ?? NO_COUNTS;
  }

  get max() {
    return Math.max(1, ...Object.values(this.counts));
  }

  // sqrt, matches map steps
  bound = (k: number) => Math.floor((this.max * k * k) / (STEPS * STEPS));

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

  setRange = (i: number) => (this.range = i);
  toggleKind = (k: Kind) =>
    (this.kinds = this.kinds.includes(k)
      ? this.kinds.filter((x) => x !== k)
      : [...this.kinds, k]);
  setDistricts = (on: boolean) => (this.districts = on);
  pick = (id: number) => (this.selected = id);
  close = () => (this.selected = 0);
  district = (name: string) => this.nav.filter({ district: name });

  <template>
    {{pageTitle "Karte"}}
    <div class="dis-karte">
      <h1 class="dis-visually-hidden">Karte</h1>
      <div class="dis-karte-bar">
        <CanonSegmented
          class="dis-karte-range"
          aria-label="Zeitraum"
          @items={{RANGES}}
          @selected={{this.range}}
          @onChange={{this.setRange}}
        />
        <div class="dis-karte-kinds" role="group" aria-label="Arten">
          {{#each this.kindItems key="kind" as |k|}}
            <button
              type="button"
              class="dis-karte-kind"
              aria-pressed={{if k.on "true" "false"}}
              {{on "click" (fn this.toggleKind k.kind)}}
            >
              <span
                class="dis-place-swatch dis-kind-{{k.kind}}"
                aria-hidden="true"
              ></span>
              {{k.label}}
              <span class="dis-karte-kind-n">{{num k.n}}</span>
            </button>
          {{/each}}
        </div>
        <CanonRow
          class="dis-karte-teile"
          @title="Stadtteile"
          @icon={{component Icon name="karte-layers"}}
          as |titleId|
        >
          <CanonSwitch
            aria-labelledby={{titleId}}
            @on={{this.districts}}
            @onChange={{this.setDistricts}}
          />
        </CanonRow>
      </div>
      {{#if this.load.error}}
        <LoadError @error={{this.load.error}} />
      {{/if}}
      <div class="dis-karte-body">
        <div class="dis-karte-stage">
          <MapView
            class="dis-karte-map"
            @places={{this.all}}
            @kinds={{this.kinds}}
            @districts={{this.districts}}
            @counts={{this.counts}}
            @selected={{this.selected}}
            @onPick={{this.pick}}
            @onDistrict={{this.district}}
          />
          {{#if this.districts}}
            <div class="dis-karte-legend" aria-hidden="true">
              <span class="dis-karte-legend-title"><Icon
                  @name="docs-files"
                  @size={{14}}
                />Dokumente</span>
              {{#each this.legend as |l|}}
                <span class="dis-karte-legend-item">
                  <span class="dis-place-swatch dis-step-{{l.step}}"></span>
                  {{l.text}}
                </span>
              {{/each}}
            </div>
          {{/if}}
          {{#if this.place}}
            <MapPanel @place={{this.place}} @onClose={{this.close}} />
          {{/if}}
        </div>
        {{#if this.load.value}}
          <MapList
            @places={{this.places}}
            @selected={{this.selected}}
            @onPick={{this.pick}}
          />
        {{else if this.load.pending}}
          <InlineLoading @text="Lädt…" />
        {{/if}}
      </div>
    </div>
  </template>
}
