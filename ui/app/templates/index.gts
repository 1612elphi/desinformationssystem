import Component from '@glimmer/component';
import { service } from '@ember/service';
import { concat, fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { pageTitle } from 'ember-page-title';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonLoader from 'delphicanon/components/canon-loader';
import CanonRow from 'delphicanon/components/canon-row';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import CanonTextCell from 'delphicanon/components/canon-text-cell';
import type { Doc } from 'ui/api';
import type IndexController from 'ui/controllers/index';
import { ALL_COLUMNS, PAGE_SIZE, VIEWS } from 'ui/controllers/index';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import ColumnPicker from 'ui/components/column-picker';
import DataTable from 'ui/components/data-table';
import DocsFacets from 'ui/components/docs-facets';
import DocsList from 'ui/components/docs-list';
import DocsWeek from 'ui/components/docs-week';
import { SelectField, DateField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { LoadError } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import { SubmitterChips } from 'ui/components/party-chip';
import Snippet from 'ui/components/snippet';
import Tag from 'ui/components/tag';
import shortcut from 'ui/modifiers/shortcut';
import {
  PUBLIC_OPTS,
  SUBMITTERS,
  SUBMITTER_ORDER,
  eq,
  num,
  or,
} from 'ui/utils/meta';
import './index.css';

const topNum = (n?: string) => (n ? `TOP ${n}` : '');
const removeLabel = (label: string) => `Entfernen: ${label}`;
const docLabel = (d: Doc) => d.label;

interface Signature {
  Args: { model: ApplicationModel; controller: IndexController };
}

export default class DokumenteTemplate extends Component<Signature> {
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
      ...present.map((c) => ({ id: c, label: SUBMITTERS[c]?.name || c })),
    ];
  }

  get districtItems() {
    return [
      { id: '', label: 'Alle Stadtteile' },
      ...(this.facets?.districts ?? []).map((d) => ({ id: d, label: d })),
    ];
  }

  get empty() {
    const s = this.args.controller.search;
    return !s.pending && !s.error && !this.args.controller.results.length;
  }

  openDoc = (d: Doc) => this.nav.open({ doc: d.id });

  focusSearch = () => document.getElementById('dis-q')?.focus();

  searchKey = (e: KeyboardEvent) => {
    if (e.key !== 'Escape') return;
    if (this.args.controller.qValue) this.args.controller.clearQ();
    else (e.target as HTMLElement).blur();
  };

  // j/k move real focus through the result rows, so Enter opens the row
  move = (step: number) => {
    const c = this.args.controller;
    const rows = [
      ...document.querySelectorAll<HTMLElement>(
        '.dis-docs-results :is(.dis-dlist-open, .dis-table-open)',
      ),
    ];
    if (!rows.length) return;
    const from = c.cursor < 0 || c.cursor >= rows.length ? -1 : c.cursor;
    const i =
      from < 0
        ? step > 0
          ? 0
          : rows.length - 1
        : Math.min(rows.length - 1, Math.max(0, from + step));
    c.setCursor(i);
    rows[i]?.focus();
    rows[i]?.closest('li, tr')?.scrollIntoView({ block: 'nearest' });
  };

  next = () => this.move(1);
  prev = () => this.move(-1);

  <template>
    {{pageTitle "Dokumente"}}
    {{#let @controller as |c|}}
      <div
        class="dis-docs"
        {{shortcut "/" this.focusSearch}}
        {{shortcut "j" this.next}}
        {{shortcut "k" this.prev}}
      >
        <header class="dis-docs-hero">
          <h1>Karlsruher Ratsdokumente</h1>
          <p>
            Durchsuchbares Archiv des Sitzungskalenders der Stadt Karlsruhe —
            täglich aktualisiert, volltextindiziert und kategorisiert.
          </p>
        </header>

        {{#if @model.stats.error}}
          <LoadError @error={{@model.stats.error}} />
        {{/if}}
        <DocsWeek @stats={{@model.stats.value}} />

        <div class="dis-docs-controls">
          <div role="search">
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
                @placeholder="Volltextsuche über alle Dokumente, Zusammenfassungen und Schlagworte…"
                @onChange={{c.setQ}}
                {{on "keydown" this.searchKey}}
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
          </div>

          {{#if @model.facets.error}}
            <LoadError @error={{@model.facets.error}} />
          {{/if}}
          <div class="dis-docs-filters">
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

        <div class="dis-docs-bar">
          <span class="dis-docs-count" role="status">
            {{if c.search.pending "Lädt…" (concat (num c.total) " Treffer")}}
          </span>
          <a
            class="dis-docs-feed"
            title="RSS-Feed neuer Dokumente (mit den aktiven Filtern)"
            href={{c.feedUrl}}
          >
            <Icon @name="rss" @size={{14}} />
            RSS
          </a>
          {{#if c.activeFilters.length}}
            <span class="dis-tags dis-docs-active">
              {{#each c.activeFilters as |f|}}
                <Tag
                  aria-label={{removeLabel f.label}}
                  @onClick={{fn c.setFilter f.k f.clear}}
                >
                  {{f.label}}
                  <Icon @name="close" @size={{12}} />
                </Tag>
              {{/each}}
            </span>
            <CanonButton
              @kind="cell"
              @text="Alle zurücksetzen"
              @onClick={{c.resetAll}}
            />
          {{/if}}
          <span class="dis-docs-viewbar">
            <kbd class="dis-docs-keys" title="Tastatur">/ · J / K</kbd>
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
        </div>

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
          <CanonEmptyRow
            @text="Keine Dokumente gefunden — Suchbegriff anpassen oder Filter zurücksetzen."
          />
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
      </div>
    {{/let}}
  </template>
}
