import Component from '@glimmer/component';
import { service } from '@ember/service';
import { concat, fn } from '@ember/helper';
import { pageTitle } from 'ember-page-title';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonLoader from 'delphicanon/components/canon-loader';
import CanonRow from 'delphicanon/components/canon-row';
import CanonTextCell from 'delphicanon/components/canon-text-cell';
import CanonTiles from 'delphicanon/components/canon-tiles';
import type { Doc } from 'ui/api';
import type IndexController from 'ui/controllers/index';
import { ALL_COLUMNS, PAGE_SIZE } from 'ui/controllers/index';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import ColumnPicker from 'ui/components/column-picker';
import DataTable from 'ui/components/data-table';
import { SelectField, DateField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { LoadError, LoadState } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import { SubmitterChips } from 'ui/components/party-chip';
import Snippet from 'ui/components/snippet';
import Tag from 'ui/components/tag';
import {
  PUBLIC_OPTS,
  SUBMITTERS,
  SUBMITTER_ORDER,
  eq,
  num,
  or,
  scrapeTime,
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

  get tiles() {
    const s = this.args.model.stats.value;
    if (!s) return [];
    return [
      { label: 'Dokumente', value: num(s.documents) },
      { label: 'Sitzungen', value: num(s.meetings) },
      { label: 'Gremien', value: num(s.bodies) },
      { label: 'Volltext', value: num(s.with_text) },
      { label: 'Analysiert', value: num(s.enriched) },
      { label: 'Letzter Abruf', value: scrapeTime(s.last_scrape) },
    ];
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

  <template>
    {{pageTitle "Dokumente"}}
    {{#let @controller as |c|}}
      <div class="dis-docs">
        <header class="dis-docs-hero">
          <h1>Karlsruher Ratsdokumente</h1>
          <p>
            Durchsuchbares Archiv des Sitzungskalenders der Stadt Karlsruhe —
            täglich aktualisiert, volltextindiziert und kategorisiert.
          </p>
        </header>

        <LoadState @load={{@model.stats}}>
          <CanonTiles class="dis-docs-tiles" @items={{this.tiles}} />
        </LoadState>

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
          <span class="dis-docs-colpick">
            <ColumnPicker
              @columns={{ALL_COLUMNS}}
              @selected={{c.cols}}
              @onChange={{c.setCols}}
            />
          </span>
        </div>

        {{#if c.search.error}}
          <LoadError @error={{c.search.error}} />
        {{else if this.empty}}
          <CanonEmptyRow
            @text="Keine Dokumente gefunden — Suchbegriff anpassen oder Filter zurücksetzen."
          />
        {{else if c.results.length}}
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
                  {{#if d.agenda_number}}<span class="dis-docs-topnum">{{topNum
                        d.agenda_number
                      }}</span>{{/if}}
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
          <Pagination
            @page={{c.currentPage}}
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
