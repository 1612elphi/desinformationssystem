import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import Tag from 'ui/components/tag';
import { modifier } from 'ember-modifier';
import shortcut from 'ui/modifiers/shortcut';
import type NavService from 'ui/services/nav';
import {
  KIND_LABEL,
  fetchPlace,
  type Place,
  type PlaceDetail,
} from 'ui/utils/api-karte';
import Load from 'ui/utils/load';
import { num } from 'ui/utils/meta';
import './map-panel.css';

const dateDe = (iso?: string) =>
  iso ? iso.split('-').reverse().join('.') : '';

interface Signature {
  Args: { place: Place; onClose: () => void };
  Element: HTMLElement;
}

export default class MapPanel extends Component<Signature> {
  @service declare nav: NavService;

  #last?: Load<PlaceDetail>;

  @cached
  get detail() {
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#last = new Load(fetchPlace(this.args.place.id), this.#last));
  }

  get kind() {
    return KIND_LABEL[this.args.place.kind];
  }

  // below the map in the narrow layout, where it would open out of sight
  reveal = modifier((el: HTMLElement, [id]: [number]) => {
    if (id) el.scrollIntoView({ block: 'nearest' });
  });

  openDoc = (id: string) => this.nav.open({ doc: id });
  openMeeting = (id: string) => this.nav.open({ meeting: id });

  <template>
    <section
      class="dis-place-panel"
      aria-labelledby="dis-place-title"
      {{shortcut "Escape" @onClose}}
      {{this.reveal @place.id}}
      ...attributes
    >
      <div class="dis-place-head">
        <span class="dis-place-kind dis-kind-{{@place.kind}}"><Icon
            @name="karte-{{@place.kind}}"
            @size={{20}}
          /></span>
        <div class="dis-place-titles">
          <h2 id="dis-place-title" class="dis-place-title">{{@place.name}}</h2>
          <span class="dis-place-meta">{{this.kind}}
            ·
            {{num @place.docs}}
            Dokumente ·
            {{num @place.mentions}}
            Nennungen</span>
        </div>
        <CanonButton
          @kind="cell"
          @icon={{component Icon name="close"}}
          @label="Schließen"
          @onClick={{@onClose}}
        />
      </div>
      <div class="dis-place-docs">
        <LoadState @load={{this.detail}}>
          {{#each this.detail.value.documents key="id" as |d|}}
            <div class="dis-place-doc">
              <button
                type="button"
                class="dis-place-open"
                {{on "click" (fn this.openDoc d.id)}}
              >
                <span class="dis-place-date">{{dateDe d.meeting_date}}</span>
                <span class="dis-place-label">{{d.label}}</span>
                {{#if d.doc_type}}<Tag>{{d.doc_type}}</Tag>{{/if}}
                {{#if d.quote}}
                  <q class="dis-place-quote">{{d.quote}}</q>
                {{/if}}
              </button>
              {{#if d.meeting_id}}
                <button
                  type="button"
                  class="dis-place-meeting"
                  {{on "click" (fn this.openMeeting d.meeting_id)}}
                >
                  <Icon @name="calendar" @size={{14}} />
                  <span>{{if d.agenda_title d.agenda_title "Sitzung"}}</span>
                </button>
              {{/if}}
            </div>
          {{else}}
            <CanonEmptyRow @text="Keine Dokumente" />
          {{/each}}
        </LoadState>
      </div>
    </section>
  </template>
}
