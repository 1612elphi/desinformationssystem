import Component from '@glimmer/component';
import { service } from '@ember/service';
import { concat, fn } from '@ember/helper';
import { on } from '@ember/modifier';
import type { TOC } from '@ember/component/template-only';
import { modifier } from 'ember-modifier';
import { pageTitle } from 'ember-page-title';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonLoader from 'delphicanon/components/canon-loader';
import CanonRow from 'delphicanon/components/canon-row';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import CanonTextCell from 'delphicanon/components/canon-text-cell';
import type { Doc } from 'ui/api';
import type SucheController from 'ui/controllers/suche';
import {
  ALL_COLUMNS,
  LIST_SIZE,
  PAGE_SIZE,
  SCOPES,
  VIEWS,
  type Scope,
} from 'ui/controllers/suche';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import ColumnPicker from 'ui/components/column-picker';
import DataTable from 'ui/components/data-table';
import DocsFacets from 'ui/components/docs-facets';
import DocsList from 'ui/components/docs-list';
import { SelectField, DateField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { LoadError } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import { SubmitterChips } from 'ui/components/party-chip';
import Snippet from 'ui/components/snippet';
import {
  CommitteeHits,
  MeetingHits,
  PersonHits,
  SpeechHits,
  VoteHits,
} from 'ui/components/suche-hits';
import Tag from 'ui/components/tag';
import rough from 'ui/modifiers/rough';
import shortcut, { topDialog } from 'ui/modifiers/shortcut';
import { partyName } from 'ui/utils/api-menschen';
import type Load from 'ui/utils/load';
import { PUBLIC_OPTS, SUBMITTER_ORDER, eq, num, or } from 'ui/utils/meta';
import './suche.css';

const topNum = (n?: string) => (n ? `TOP ${n}` : '');
const removeLabel = (label: string) => `Entfernen: ${label}`;
const docLabel = (d: Doc) => d.label;
const stagger = (i: number) => 600 + i * 150;
const scopeKey = (i: number) => String(i + 1);
const countOf = (counts: Partial<Record<Scope, number>>, s: Scope) => counts[s];
const isNum = (n?: number) => n !== undefined;
const above = (a: number | undefined, b: number) => (a ?? 0) > b;
const list = <T,>(xs?: T[]) => xs ?? [];
const missing = (v: unknown) => v === undefined;
const n0 = (n?: number) => n ?? 0;
const pageOf = <T,>(list: T[], page: number) =>
  list.slice((page - 1) * LIST_SIZE, page * LIST_SIZE);

// hits per kind under Alles
const TOP = 5;
const TOP_SPEECHES = 3;
const head = <T,>(list: T[] | undefined, n = TOP) => (list ?? []).slice(0, n);

const HITS =
  '.dis-suche-results :is(.dis-dlist-open, .dis-table-open, .dis-hit-open)';

// a touch keyboard would cover the results it was opened for
const autofocus = modifier((el: HTMLElement) => {
  if (!topDialog() && matchMedia('(hover: hover)').matches)
    el.focus({ preventScroll: true });
});

const Group: TOC<{
  Args: {
    label: string;
    icon: string;
    total?: number;
    shown: number;
    onMore: () => void;
  };
  Blocks: { default: [] };
}> = <template>
  <section class="dis-suche-group" aria-label={{@label}}>
    <header class="dis-suche-group-head">
      <h2><Icon @name={{@icon}} />{{@label}}</h2>
      {{#if (isNum @total)}}
        <span class="dis-suche-group-n">{{num @total}}</span>
      {{/if}}
      {{#if (above @total @shown)}}
        <CanonButton
          class="dis-suche-more"
          @kind="cell"
          @text="mehr"
          @icon={{component Icon name="suche-more"}}
          @onClick={{@onMore}}
        />
      {{/if}}
    </header>
    {{yield}}
  </section>
</template>;

// one scope's full list: its error, a loader, an empty row, or the rows and pages
const Listing: TOC<{
  Args: {
    load: Load<unknown>;
    total?: number;
    page: number;
    onPage: (page: number) => void;
  };
  Blocks: { default: [] };
}> = <template>
  {{#if @load.error}}
    <LoadError @error={{@load.error}} />
  {{else if (missing @load.value)}}
    <div class="dis-docs-loading"><CanonLoader /></div>
  {{else if @total}}
    {{yield}}
    <Pagination
      @page={{@page}}
      @pageSize={{LIST_SIZE}}
      @total={{n0 @total}}
      @onChange={{@onPage}}
    />
  {{else}}
    <div class="dis-docs-none">
      <Icon @name="suche-none" @size={{20}} />
      <CanonEmptyRow @text="Keine Treffer" />
    </div>
  {{/if}}
</template>;

interface Signature {
  Args: { model: ApplicationModel; controller: SucheController };
}

export default class SucheTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get facets() {
    return this.args.model.facets.value;
  }

  get committeeItems() {
    return [
      { id: '', label: 'Alle Gremien' },
      ...(this.facets?.committees ?? []).map((c) => ({
        id: c.name,
        label: c.name,
      })),
    ];
  }

  get typeItems() {
    return [
      { id: '', label: 'Alle Dokumenttypen' },
      ...(this.facets?.doc_types ?? []).map((t) => ({ id: t, label: t })),
    ];
  }

  get submitterItems() {
    const rank = (c: string) => SUBMITTER_ORDER.indexOf(c) + 1 || 99;
    const present = [...(this.facets?.submitters ?? [])].sort(
      (a, b) => rank(a) - rank(b),
    );
    return [
      { id: '', label: 'Alle Einbringer' },
      ...present.map((c) => ({ id: c, label: partyName(c) })),
    ];
  }

  get districtItems() {
    return [
      { id: '', label: 'Alle Stadtteile' },
      ...(this.facets?.districts ?? []).map((d) => ({ id: d, label: d })),
    ];
  }

  get showBar() {
    const c = this.args.controller;
    return c.scope === 'dokumente' || c.activeFilters.length > 0;
  }

  get empty() {
    const s = this.args.controller.search;
    return !s.pending && !s.error && !this.args.controller.results.length;
  }

  // Alles: names only match a query, so people and bodies wait for one
  get named() {
    return this.args.controller.q.trim() !== '';
  }

  get allesDocs() {
    return head(this.args.controller.results);
  }

  get allesMeetings() {
    return head(this.args.controller.meetings.value?.results);
  }

  get allesPeople() {
    return this.named ? head(this.args.controller.personHits) : [];
  }

  get allesCommittees() {
    return this.named ? head(this.args.controller.committeeHits) : [];
  }

  get allesSpeeches() {
    return head(this.args.controller.speeches.value?.results, TOP_SPEECHES);
  }

  get allesVotes() {
    return head(this.args.controller.votes.value?.results);
  }

  get allesLoads() {
    const c = this.args.controller;
    return [c.search, c.meetings, c.speeches, c.votes, c.people, c.committees];
  }

  get allesErrors() {
    return [...new Set(this.allesLoads.map((l) => l.error).filter(Boolean))];
  }

  get allesShown() {
    return (
      this.allesDocs.length +
      this.allesMeetings.length +
      this.allesPeople.length +
      this.allesCommittees.length +
      this.allesSpeeches.length +
      this.allesVotes.length
    );
  }

  get allesPending() {
    return this.allesLoads.some((l) => l.pending);
  }

  get pagedPeople() {
    const c = this.args.controller;
    return pageOf(c.personHits, c.listPage);
  }

  get pagedCommittees() {
    const c = this.args.controller;
    return pageOf(c.committeeHits, c.listPage);
  }

  openDoc = (d: Doc) => this.nav.open({ doc: d.id });
  openMeeting = (id: string) => this.nav.open({ meeting: id });
  openPerson = (id: string) => this.nav.open({ person: id });
  openCommittee = (id: string) => this.nav.go('gremien', { gremium: id });

  focusSearch = () => document.getElementById('dis-q')?.focus();

  searchKey = (e: KeyboardEvent) => {
    if (e.key !== 'Escape') return;
    if (this.args.controller.qValue) this.args.controller.clearQ();
    else (e.target as HTMLElement).blur();
  };

  // j/k move real focus through the hits of every group, so Enter opens the row
  move = (step: number) => {
    const c = this.args.controller;
    const rows = [...document.querySelectorAll<HTMLElement>(HITS)];
    if (!rows.length) return;
    let from = rows.indexOf(document.activeElement as HTMLElement);
    if (from < 0 && c.scope === 'dokumente' && c.cursor < rows.length)
      from = c.cursor;
    const i =
      from < 0
        ? step > 0
          ? 0
          : rows.length - 1
        : Math.min(rows.length - 1, Math.max(0, from + step));
    if (c.scope === 'dokumente') c.setCursor(i);
    rows[i]?.focus();
    rows[i]?.closest('li, tr')?.scrollIntoView({ block: 'nearest' });
  };

  next = () => this.move(1);
  prev = () => this.move(-1);

  <template>
    {{pageTitle "Suche"}}
    {{#let @controller as |c|}}
      <div
        class="dis-docs dis-suche"
        {{shortcut "/" this.focusSearch}}
        {{shortcut "j" this.next}}
        {{shortcut "k" this.prev}}
      >
        <h1 class="dis-visually-hidden">Suche</h1>
        <div class="dis-suche-top" role="search">
          <CanonRow
            class="dis-docs-search"
            @title="Suche"
            @form="flush"
            as |titleId|
          >
            <CanonTextCell
              id="dis-q"
              autocomplete="off"
              aria-labelledby={{titleId}}
              @value={{c.qValue}}
              @placeholder="Suchen…"
              @onChange={{c.setQ}}
              {{on "keydown" this.searchKey}}
              {{autofocus}}
            />
            {{#if c.qValue}}
              <CanonButton
                class="dis-docs-clear"
                @kind="cell"
                @icon={{component Icon name="close"}}
                @label="Suche leeren"
                @onClick={{c.clearQ}}
              />
            {{/if}}
          </CanonRow>
          <div class="dis-suche-scopes" role="group" aria-label="Bereich">
            {{#each SCOPES as |s i|}}
              <button
                type="button"
                class="dis-suche-scope"
                aria-pressed={{if (eq c.scope s.id) "true" "false"}}
                {{on "click" (fn c.setScope s.id)}}
                {{shortcut (scopeKey i) (fn c.setScope s.id)}}
              >
                <Icon @name={{s.icon}} />
                <span class="dis-suche-scope-label"><span
                    {{rough
                      "underline"
                      show=(eq c.scope s.id)
                      strokeWidth=2
                      padding=1
                    }}
                  >{{s.label}}</span></span>
                {{#let (countOf c.counts s.id) as |n|}}
                  {{#if (isNum n)}}
                    <span
                      class="dis-suche-scope-n
                        {{unless n 'dis-suche-scope-n--none'}}"
                    >{{num n}}</span>
                  {{/if}}
                {{/let}}
                <kbd class="dis-suche-scope-key">{{scopeKey i}}</kbd>
              </button>
            {{/each}}
          </div>
        </div>

        {{#if (eq c.scope "dokumente")}}
          {{#if c.filtersOpen}}
            {{#if @model.facets.error}}
              <LoadError @error={{@model.facets.error}} />
            {{/if}}
            <div class="dis-docs-controls">
              <div class="dis-docs-filters" id="dis-suche-filters">
                <SelectField
                  @label="Gremium"
                  @items={{this.committeeItems}}
                  @value={{c.committee}}
                  @onChange={{fn c.setFilter "committee"}}
                />
                <SelectField
                  @label="Dokumenttyp"
                  @items={{this.typeItems}}
                  @value={{c.type}}
                  @onChange={{fn c.setFilter "type"}}
                />
                <SelectField
                  @label="Einbringer"
                  @items={{this.submitterItems}}
                  @value={{c.sub}}
                  @onChange={{fn c.setFilter "sub"}}
                />
                <SelectField
                  @label="Stadtteil"
                  @items={{this.districtItems}}
                  @value={{c.district}}
                  @onChange={{fn c.setFilter "district"}}
                />
                <SelectField
                  @label="Öffentlichkeit"
                  @items={{PUBLIC_OPTS}}
                  @value={{c.pub}}
                  @onChange={{fn c.setFilter "pub"}}
                />
                <DateField
                  @id="dis-from"
                  @label="Von"
                  @value={{c.from}}
                  @onChange={{fn c.setFilter "from"}}
                />
                <DateField
                  @id="dis-to"
                  @label="Bis"
                  @value={{c.to}}
                  @onChange={{fn c.setFilter "to"}}
                />
              </div>
            </div>
          {{/if}}
        {{/if}}

        {{#if this.showBar}}
          <div class="dis-docs-bar">
            {{#if (eq c.scope "dokumente")}}
              <span class="dis-docs-count" role="status">
                <Icon @name="docs-files" />
                {{if
                  c.search.pending
                  "Lädt…"
                  (concat (num c.total) " Treffer")
                }}
              </span>
              <CanonButton
                class="dis-suche-filter-toggle"
                aria-expanded={{if c.filtersOpen "true" "false"}}
                aria-controls="dis-suche-filters"
                @kind="cell"
                @selected={{c.filtersOpen}}
                @icon={{component Icon name="suche-filters"}}
                @text="Filter"
                @onClick={{c.toggleFilters}}
              />
              <a
                class="dis-docs-feed"
                title="RSS-Feed neuer Dokumente (mit den aktiven Filtern)"
                href={{c.feedUrl}}
              >
                <Icon @name="rss" @size={{14}} />
                RSS
              </a>
            {{/if}}
            {{#if c.activeFilters.length}}
              <span class="dis-tags dis-docs-active">
                <Icon @name="docs-filter" />
                {{#each c.activeFilters as |f i|}}
                  <Tag
                    aria-label={{removeLabel f.label}}
                    {{rough
                      "underline"
                      color="--dis-highlight"
                      padding=2
                      delay=(stagger i)
                    }}
                    @onClick={{fn c.setFilter f.k f.clear}}
                  >
                    {{f.label}}
                    <Icon @name="close" @size={{12}} />
                  </Tag>
                {{/each}}
              </span>
              <CanonButton
                @kind="cell"
                @icon={{component Icon name="docs-reset"}}
                @text="Alle zurücksetzen"
                @onClick={{c.resetAll}}
              />
            {{/if}}
            {{#if (eq c.scope "dokumente")}}
              <span class="dis-docs-viewbar">
                {{#if (eq c.view 1)}}
                  <span class="dis-docs-colpick">
                    <ColumnPicker
                      @columns={{ALL_COLUMNS}}
                      @selected={{c.cols}}
                      @onChange={{c.setCols}}
                    />
                  </span>
                {{/if}}
                <CanonSegmented
                  class="dis-docs-view"
                  aria-label="Ansicht"
                  @items={{VIEWS}}
                  @selected={{c.view}}
                  @boxed={{true}}
                  @onChange={{c.setView}}
                />
              </span>
            {{/if}}
          </div>
        {{/if}}

        <div class="dis-suche-results">
          {{#if (eq c.scope "alles")}}
            {{#each this.allesErrors as |e|}}
              <LoadError @error={{e}} />
            {{/each}}
            {{#if this.allesPeople.length}}
              <Group
                @label="Personen"
                @icon="suche-people"
                @total={{c.counts.personen}}
                @shown={{this.allesPeople.length}}
                @onMore={{fn c.setScope "personen"}}
              >
                <PersonHits
                  @people={{this.allesPeople}}
                  @onOpen={{this.openPerson}}
                />
              </Group>
            {{/if}}
            {{#if this.allesCommittees.length}}
              <Group
                @label="Gremien"
                @icon="suche-bodies"
                @total={{c.counts.gremien}}
                @shown={{this.allesCommittees.length}}
                @onMore={{fn c.setScope "gremien"}}
              >
                <CommitteeHits
                  @committees={{this.allesCommittees}}
                  @onOpen={{this.openCommittee}}
                />
              </Group>
            {{/if}}
            {{#if this.allesDocs.length}}
              <Group
                @label="Dokumente"
                @icon="suche-docs"
                @total={{c.counts.dokumente}}
                @shown={{this.allesDocs.length}}
                @onMore={{fn c.setScope "dokumente"}}
              >
                <DocsList
                  @docs={{this.allesDocs}}
                  @cursor={{-1}}
                  @onOpen={{this.openDoc}}
                  @onFocusRow={{c.setCursor}}
                />
              </Group>
            {{/if}}
            {{#if this.allesVotes.length}}
              <Group
                @label="Abstimmungen"
                @icon="suche-votes"
                @total={{c.counts.abstimmungen}}
                @shown={{this.allesVotes.length}}
                @onMore={{fn c.setScope "abstimmungen"}}
              >
                <VoteHits
                  @votes={{this.allesVotes}}
                  @q={{c.q}}
                  @onOpen={{this.openMeeting}}
                />
              </Group>
            {{/if}}
            {{#if this.allesMeetings.length}}
              <Group
                @label="Sitzungen"
                @icon="suche-meetings"
                @total={{c.counts.sitzungen}}
                @shown={{this.allesMeetings.length}}
                @onMore={{fn c.setScope "sitzungen"}}
              >
                <MeetingHits
                  @meetings={{this.allesMeetings}}
                  @onOpen={{this.openMeeting}}
                />
              </Group>
            {{/if}}
            {{#if this.allesSpeeches.length}}
              <Group
                @label="Reden"
                @icon="suche-speeches"
                @total={{c.counts.reden}}
                @shown={{this.allesSpeeches.length}}
                @onMore={{fn c.setScope "reden"}}
              >
                <SpeechHits
                  @speeches={{this.allesSpeeches}}
                  @q={{c.q}}
                  @personIds={{c.personIds}}
                  @onOpen={{this.openMeeting}}
                  @onPerson={{this.openPerson}}
                />
              </Group>
            {{/if}}
            {{#if this.allesPending}}
              <div class="dis-docs-loading"><CanonLoader /></div>
            {{else if (eq this.allesShown 0)}}
              <div class="dis-docs-none">
                <Icon @name="suche-none" @size={{20}} />
                <CanonEmptyRow @text="Keine Treffer" />
              </div>
            {{/if}}
          {{else if (eq c.scope "sitzungen")}}
            <Listing
              @load={{c.meetings}}
              @total={{c.meetings.value.total}}
              @page={{c.listPage}}
              @onPage={{c.setListPage}}
            >
              <MeetingHits
                @meetings={{list c.meetings.value.results}}
                @onOpen={{this.openMeeting}}
              />
            </Listing>
          {{else if (eq c.scope "personen")}}
            <Listing
              @load={{c.people}}
              @total={{c.personHits.length}}
              @page={{c.listPage}}
              @onPage={{c.setListPage}}
            >
              <PersonHits
                @people={{this.pagedPeople}}
                @onOpen={{this.openPerson}}
              />
            </Listing>
          {{else if (eq c.scope "gremien")}}
            <Listing
              @load={{c.committees}}
              @total={{c.committeeHits.length}}
              @page={{c.listPage}}
              @onPage={{c.setListPage}}
            >
              <CommitteeHits
                @committees={{this.pagedCommittees}}
                @onOpen={{this.openCommittee}}
              />
            </Listing>
          {{else if (eq c.scope "reden")}}
            <Listing
              @load={{c.speeches}}
              @total={{c.speeches.value.total}}
              @page={{c.listPage}}
              @onPage={{c.setListPage}}
            >
              <SpeechHits
                @speeches={{list c.speeches.value.results}}
                @q={{c.q}}
                @personIds={{c.personIds}}
                @onOpen={{this.openMeeting}}
                @onPerson={{this.openPerson}}
              />
            </Listing>
          {{else if (eq c.scope "abstimmungen")}}
            <Listing
              @load={{c.votes}}
              @total={{c.votes.value.total}}
              @page={{c.listPage}}
              @onPage={{c.setListPage}}
            >
              <VoteHits
                @votes={{list c.votes.value.results}}
                @q={{c.q}}
                @onOpen={{this.openMeeting}}
              />
            </Listing>
          {{else}}
            {{#if c.facetCounts}}
              <DocsFacets
                @categories={{c.facetCounts.category}}
                @types={{c.facetCounts.doc_type}}
                @category={{c.cat}}
                @type={{c.type}}
                @onCategory={{fn c.setFilter "cat"}}
                @onType={{fn c.setFilter "type"}}
              />
            {{/if}}

            {{#if c.search.error}}
              <LoadError @error={{c.search.error}} />
            {{else if this.empty}}
              <div class="dis-docs-none">
                <Icon @name="docs-none" @size={{20}} />
                <CanonEmptyRow
                  @text="Keine Dokumente gefunden — Suchbegriff anpassen oder Filter zurücksetzen."
                />
              </div>
            {{else if c.results.length}}
              <div class="dis-docs-results">
                {{#if (eq c.view 0)}}
                  <DocsList
                    @docs={{c.results}}
                    @cursor={{c.cursor}}
                    @onOpen={{this.openDoc}}
                    @onFocusRow={{c.setCursor}}
                  />
                {{else}}
                  <DataTable
                    class="dis-docs-table"
                    @columns={{c.visibleColumns}}
                    @rows={{c.results}}
                    @caption="Dokumente"
                    @onOpen={{this.openDoc}}
                    @rowLabel={{docLabel}}
                    as |d col|
                  >
                    {{#if (eq col.key "date")}}
                      {{if d.meeting_date d.meeting_date "—"}}
                    {{else if (eq col.key "gremium")}}
                      {{if d.body_name d.body_name "—"}}
                    {{else if (eq col.key "top")}}
                      {{#if (or d.agenda_number d.agenda_title)}}
                        <span class="dis-docs-top" title={{d.agenda_title}}>
                          {{#if d.agenda_number}}<span
                              class="dis-docs-topnum"
                            >{{topNum d.agenda_number}}</span>{{/if}}
                          {{d.agenda_title}}
                        </span>
                      {{else}}
                        —
                      {{/if}}
                    {{else if (eq col.key "sub")}}
                      <SubmitterChips @codes={{d.submitters}} />
                    {{else if (eq col.key "typ")}}
                      <Tag>{{d.doc_type}}</Tag>
                    {{else if (eq col.key "doc")}}
                      <span class="dis-docs-doc">
                        <span class="dis-docs-label">{{d.label}}</span>
                        {{#if d.snippet}}<Snippet @text={{d.snippet}} />{{/if}}
                      </span>
                    {{else if (eq col.key "cat")}}
                      {{#if d.category}}<Tag>{{d.category}}</Tag>{{/if}}
                    {{else if (eq col.key "stadtteil")}}
                      {{#if d.district}}<Tag>{{d.district}}</Tag>{{/if}}
                    {{/if}}
                  </DataTable>
                {{/if}}
              </div>
              <Pagination
                @page={{c.page}}
                @pageSize={{PAGE_SIZE}}
                @total={{c.total}}
                @onChange={{c.setPage}}
              />
            {{else}}
              <div class="dis-docs-loading"><CanonLoader /></div>
            {{/if}}
          {{/if}}
        </div>

        <kbd class="dis-docs-keys dis-suche-keys" title="Tastatur"><Icon
            @name="docs-keys"
            @size={{14}}
          />
          / · J / K · ↵ · 1–7</kbd>
      </div>
    {{/let}}
  </template>
}
