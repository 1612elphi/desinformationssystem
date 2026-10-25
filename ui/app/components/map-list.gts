import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonRow from 'delphicanon/components/canon-row';
import CanonTextCell from 'delphicanon/components/canon-text-cell';
import shortcut from 'ui/modifiers/shortcut';
import { KIND_LABEL, type Place } from 'ui/utils/api-karte';
import { matches } from 'ui/utils/api-suche';
import { eq, num } from 'ui/utils/meta';
import './map-list.css';

// ponytail: a capped list; the search reaches every place past the cap
const CAP = 200;
const FIELD = 'dis-place-filter';

interface Signature {
  Args: { places: Place[]; selected: number; onPick: (id: number) => void };
  Element: HTMLElement;
}

export default class MapList extends Component<Signature> {
  @tracked query = '';

  get rows() {
    const q = this.query.trim();
    const hits = q
      ? this.args.places.filter((p) => matches(q, [p.name, KIND_LABEL[p.kind]]))
      : this.args.places;
    return hits.slice(0, CAP);
  }

  setQuery = (v: string) => (this.query = v);
  focusField = (e: KeyboardEvent) => {
    e.preventDefault();
    document.getElementById(FIELD)?.focus();
  };

  // Enter in the field picks the first hit
  fieldKey = (e: KeyboardEvent) => {
    const first = this.rows[0];
    if (e.key === 'Enter' && first) this.args.onPick(first.id);
  };

  label = (p: Place) =>
    `${p.name}, ${KIND_LABEL[p.kind]}: ${num(p.docs)} Dokumente`;

  <template>
    <nav class="dis-place-list" aria-label="Orte" ...attributes>
      <CanonRow
        class="dis-place-filter"
        @title="Orte"
        @form="flush"
        {{shortcut "/" this.focusField}}
        as |titleId|
      >
        <CanonTextCell
          id={{FIELD}}
          autocomplete="off"
          aria-labelledby={{titleId}}
          aria-keyshortcuts="/"
          @value={{this.query}}
          @placeholder="/"
          @onChange={{this.setQuery}}
          {{on "keydown" this.fieldKey}}
        />
      </CanonRow>
      <ul class="dis-place-rows">
        {{#each this.rows key="id" as |p|}}
          <li>
            <button
              type="button"
              class="dis-place-row"
              aria-label={{this.label p}}
              aria-pressed={{if (eq @selected p.id) "true" "false"}}
              {{on "click" (fn @onPick p.id)}}
            >
              <span
                class="dis-place-swatch dis-kind-{{p.kind}}"
                aria-hidden="true"
              ></span>
              <span class="dis-place-name">{{p.name}}</span>
              <span class="dis-place-n">{{num p.docs}}</span>
            </button>
          </li>
        {{else}}
          <li><CanonEmptyRow @text="Keine Orte" /></li>
        {{/each}}
      </ul>
    </nav>
  </template>
}
