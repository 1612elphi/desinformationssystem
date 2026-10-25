import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import { pageTitle } from 'ember-page-title';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonSplit from 'delphicanon/components/canon-split';
import CanonTextCell from 'delphicanon/components/canon-text-cell';
import CanonTiles from 'delphicanon/components/canon-tiles';
import 'delphicanon/components/canon-heading.css';
import type GremienController from 'ui/controllers/gremien';
import type NavService from 'ui/services/nav';
import DocRow from 'ui/components/doc-row';
import GremiumMembers from 'ui/components/gremium-members';
import GremiumShare from 'ui/components/gremium-share';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import SeatChart from 'ui/components/seat-chart';
import rough from 'ui/modifiers/rough';
import shortcut from 'ui/modifiers/shortcut';
import { daysUntil, deDate } from 'ui/utils/api-menschen';
import { iso } from 'ui/utils/calendar';
import { eq, num } from 'ui/utils/meta';
import './gremien.css';

const GROUP_ICON: Record<string, string> = {
  Gemeinderat: 'landmark',
  Ausschüsse: 'users',
  Ortschaftsräte: 'map',
  Weitere: 'building-2',
};
const groupIcon = (name: string) => GROUP_ICON[name] ?? 'building-2';

const TITLE_ID = 'dis-gremium-title';
const FILTER_ID = 'dis-gremien-q';
const ITEM = '.dis-gremien-item';

interface Signature {
  Args: { controller: GremienController };
}

export default class GremienTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get selectedName() {
    const c = this.args.controller;
    return (
      c.list.value?.find((g) => g.id === c.currentId)?.name ??
      c.selected?.value?.name ??
      ''
    );
  }

  get upcoming() {
    const today = iso(new Date());
    return (this.args.controller.selected?.value?.meetings ?? [])
      .filter((m) => (m.date ?? '') >= today)
      .sort((a, b) => (a.date ?? '').localeCompare(b.date ?? ''))
      .slice(0, 4);
  }

  get lastMeeting() {
    const today = iso(new Date());
    return this.args.controller.selected?.value?.meetings.find(
      (m) => (m.date ?? '') < today,
    );
  }

  get tiles() {
    const c = this.args.controller;
    const sel = c.selected?.value;
    if (!sel) return [];
    // the committee payload caps meetings at 300; the list has the full count
    const meetings =
      c.list.value?.find((g) => g.id === c.currentId)?.meetings ??
      sel.meetings.length;
    return [
      { label: 'Mitglieder', value: num(sel.members.length) },
      { label: 'Sitzungen', value: num(meetings) },
      { label: 'Dokumente', value: num(sel.document_count) },
      {
        label: 'Nächste Sitzung',
        value: this.upcoming[0] ? deDate(this.upcoming[0].date) : '—',
      },
    ];
  }

  get seatLabel() {
    return `Sitzverteilung ${this.selectedName}`;
  }

  // in one column the detail sits below the list, out of sight
  pick = (id: string) => {
    this.args.controller.select(id);
    requestAnimationFrame(() => {
      const title = document.getElementById(TITLE_ID);
      if (!title) return;
      const r = title.getBoundingClientRect();
      if (r.top >= 0 && r.bottom <= window.innerHeight) return;
      title.scrollIntoView({ block: 'start' });
      title.focus({ preventScroll: true });
    });
  };

  openPerson = (id: string) => this.nav.open({ person: id });
  openMeeting = (id: string) => this.nav.open({ meeting: id });
  openDoc = (id: string) => this.nav.open({ doc: id });

  focusFilter = () => document.getElementById(FILTER_ID)?.focus();

  items = () => [...document.querySelectorAll<HTMLButtonElement>(ITEM)];

  move = (step: number) => {
    const items = this.items();
    if (!items.length) return;
    let i = items.indexOf(document.activeElement as HTMLButtonElement);
    if (i < 0) i = items.findIndex((b) => b.ariaCurrent === 'true');
    const next =
      items[i < 0 ? 0 : Math.max(0, Math.min(items.length - 1, i + step))]!;
    next.focus();
    next.scrollIntoView({ block: 'nearest' });
  };

  next = () => this.move(1);
  prev = () => this.move(-1);

  filterKey = (e: KeyboardEvent) => {
    const c = this.args.controller;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.items()[0]?.focus();
    } else if (e.key === 'Enter' && c.visible[0]) {
      e.preventDefault();
      this.pick(c.visible[0].id);
    } else if (e.key === 'Escape' && c.query) {
      e.preventDefault();
      c.setQuery('');
    }
  };

  <template>
    {{pageTitle "Gremien"}}
    <h1 class="dis-visually-hidden">Gremien</h1>
    {{#let @controller as |c|}}
      <CanonSplit class="dis-gremien" @sidebarWidth={{380}}>
        <:sidebar>
          <CanonRows
            @framed={{true}}
            {{shortcut "/" this.focusFilter}}
            {{shortcut "j" this.next}}
            {{shortcut "k" this.prev}}
          >
            <h2 class="canon-heading dis-gremien-heading"><Icon
                @name="landmark"
                @size={{14}}
              />
              Gremien
              {{#if c.list.value}}({{num c.visible.length}}){{/if}}</h2>
            <CanonRow
              class="dis-gremien-filter"
              @title="Filter"
              @form="flush"
              as |titleId|
            >
              <CanonTextCell
                id={{FILTER_ID}}
                autocomplete="off"
                aria-labelledby={{titleId}}
                aria-keyshortcuts="/"
                @value={{c.query}}
                @placeholder="/"
                @onChange={{c.setQuery}}
                {{on "keydown" this.filterKey}}
              />
            </CanonRow>
            <LoadState @load={{c.list}}>
              {{#each c.groups key="name" as |g|}}
                <h3 class="canon-heading dis-gremien-group">
                  <span class="dis-gremien-heading"><Icon
                      @name={{groupIcon g.name}}
                      @size={{14}}
                    />{{g.name}}</span>
                  <span class="dis-gremien-meta">{{num g.items.length}}</span>
                </h3>
                {{#each g.items key="id" as |item|}}
                  <button
                    type="button"
                    class="dis-gremien-item
                      {{unless item.members 'dis-gremien-item--empty'}}"
                    aria-current={{if (eq c.currentId item.id) "true"}}
                    {{on "click" (fn this.pick item.id)}}
                  >
                    <span>{{item.name}}</span>
                    <span class="dis-gremien-meta">
                      {{num item.members}}
                      Mitglieder ·
                      {{num item.meetings}}
                      Sitzungen ·
                      {{num item.documents}}
                      Dok.
                    </span>
                  </button>
                {{/each}}
              {{else}}
                <CanonEmptyRow @text="Keine Treffer" />
              {{/each}}
            </LoadState>
            <p class="dis-gremien-keys" aria-hidden="true">
              <Icon @name="keyboard" @size={{14}} />
              <kbd>/</kbd>
              Filter ·
              <kbd>J</kbd>
              <kbd>K</kbd>
              Auswahl</p>
          </CanonRows>
        </:sidebar>
        <:main>
          {{#if c.currentId}}
            <CanonRows class="dis-gremien-detail" @framed={{true}}>
              <h2 id={{TITLE_ID}} class="canon-heading" tabindex="-1"><span
                  class="dis-gremien-title"
                  {{rough
                    "underline"
                    color="--dis-highlight"
                    strokeWidth=2.5
                    padding=2
                  }}
                >{{if
                    this.selectedName
                    this.selectedName
                    "Gremium"
                  }}</span></h2>
              <LoadState @load={{c.selected}}>
                {{#let c.selected.value as |sel|}}
                  <CanonTiles @items={{this.tiles}} />

                  {{#if c.members.length}}
                    <h3 class="canon-heading dis-gremien-heading"><Icon
                        @name="armchair"
                        @size={{14}}
                      />Zusammensetzung</h3>
                    <div class="dis-gremien-chamber">
                      <SeatChart
                        @seats={{c.members}}
                        @mode="party"
                        @label={{this.seatLabel}}
                        @onPick={{this.openPerson}}
                      />
                    </div>
                    <GremiumShare @parts={{sel.composition}} />

                    <h3 class="canon-heading dis-gremien-heading"><Icon
                        @name="users"
                        @size={{14}}
                      />Mitglieder ({{num c.members.length}})</h3>
                    <GremiumMembers
                      @members={{c.members}}
                      @onOpen={{this.openPerson}}
                    />
                  {{/if}}

                  <h3 class="canon-heading dis-gremien-heading"><Icon
                      @name="calendar-clock"
                      @size={{14}}
                    />Nächste Sitzungen</h3>
                  {{#each this.upcoming key="id" as |m i|}}
                    <button
                      type="button"
                      class="dis-gremien-row"
                      {{on "click" (fn this.openMeeting m.id)}}
                    >
                      <span
                        class="dis-gremien-date"
                        {{rough "circle" show=(eq i 0) padding=6 delay=500}}
                      >{{deDate m.date true}}</span>
                      <span class="dis-gremien-row-title">{{if
                          m.title
                          m.title
                          sel.name
                        }}</span>
                      <span class="dis-gremien-meta">{{daysUntil m.date}}{{#if
                          m.time
                        }}
                          ·
                          {{m.time}}{{/if}}</span>
                    </button>
                  {{else}}
                    {{#if this.lastMeeting}}
                      <button
                        type="button"
                        class="dis-gremien-row"
                        {{on "click" (fn this.openMeeting this.lastMeeting.id)}}
                      >
                        <span class="dis-gremien-date">{{deDate
                            this.lastMeeting.date
                            true
                          }}</span>
                        <span class="dis-gremien-row-title">{{if
                            this.lastMeeting.title
                            this.lastMeeting.title
                            sel.name
                          }}</span>
                        <span class="dis-gremien-meta">Letzte Sitzung</span>
                      </button>
                    {{else}}
                      <CanonEmptyRow @text="Keine Termine" />
                    {{/if}}
                  {{/each}}

                  {{#if sel.recent_documents.length}}
                    <h3 class="canon-heading dis-gremien-heading"><Icon
                        @name="file-plus"
                        @size={{14}}
                      />Neue Dokumente</h3>
                    {{#each sel.recent_documents key="id" as |d|}}
                      <div class="dis-gremien-doc">
                        <span class="dis-gremien-date">{{deDate
                            d.meeting_date
                          }}</span>
                        <DocRow
                          @doc={{d}}
                          @submitters={{true}}
                          @onOpen={{fn this.openDoc d.id}}
                        />
                      </div>
                    {{/each}}
                  {{/if}}
                {{/let}}
              </LoadState>
            </CanonRows>
          {{else}}
            <CanonRows @framed={{true}}>
              <CanonEmptyRow
                @text="Gremium auswählen, um Mitglieder und Zusammensetzung zu sehen."
              />
            </CanonRows>
          {{/if}}
        </:main>
      </CanonSplit>
    {{/let}}
  </template>
}
