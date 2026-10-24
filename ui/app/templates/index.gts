import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { concat, fn } from '@ember/helper';
import type IndexController from 'ui/controllers/index';
import { ALL_COLUMNS, PAGE_SIZE } from 'ui/controllers/index';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import ColumnPicker from 'ui/components/column-picker';
import { SelectField, DateField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { TableSkeleton } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import { SubmitterChips } from 'ui/components/party-chip';
import Snippet from 'ui/components/snippet';
import Tag from 'ui/components/tag';
import {
  PUBLIC_OPTS,
  SUBMITTERS,
  SUBMITTER_ORDER,
  eq,
  inputValue,
  num,
} from 'ui/utils/meta';

const topNum = (n?: string) => (n ? `TOP ${n}` : '');
const topFull = (n?: string, title?: string) =>
  [topNum(n), title || ''].filter(Boolean).join(' — ');

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

  get skeletonColumns() {
    return this.args.controller.visibleColumns.length || 1;
  }

  openDoc = (id: string) => this.nav.open({ doc: id });

  <template>
    {{#let @controller as |c|}}
      <div class="dis-panel">
        <div class="cds--form-item">
          <div class="cds--search cds--search--lg" role="search">
            <label
              for="dis-q"
              class="cds--label cds--visually-hidden"
            >Suche</label>
            <div class="cds--search-magnifier" aria-hidden="true">
              <Icon @name="search" class="cds--search-magnifier-icon" />
            </div>
            <input
              id="dis-q"
              type="text"
              autocomplete="off"
              class="cds--search-input"
              placeholder="Volltextsuche über alle Dokumente, Zusammenfassungen und Schlagworte…"
              value={{c.qValue}}
              {{on "input" (inputValue c.setQ)}}
            />
            <button
              type="button"
              class="cds--search-close
                {{unless c.qValue 'cds--search-close--hidden'}}"
              aria-label="Clear search input"
              {{on "click" c.clearQ}}
            >
              <Icon @name="close" />
            </button>
          </div>
        </div>

        <div class="dis-filters">
          <SelectField
            @id="committee"
            @label="Gremium"
            @items={{this.committeeItems}}
            @value={{c.committee}}
            @onChange={{fn c.setFilter "committee"}}
          />
          <SelectField
            @id="doctype"
            @label="Dokumenttyp"
            @items={{this.typeItems}}
            @value={{c.type}}
            @onChange={{fn c.setFilter "type"}}
          />
          <SelectField
            @id="submitter"
            @label="Einbringer"
            @items={{this.submitterItems}}
            @value={{c.sub}}
            @onChange={{fn c.setFilter "sub"}}
          />
          <SelectField
            @id="district"
            @label="Stadtteil"
            @items={{this.districtItems}}
            @value={{c.district}}
            @onChange={{fn c.setFilter "district"}}
          />
          <SelectField
            @id="public"
            @label="Öffentlichkeit"
            @items={{PUBLIC_OPTS}}
            @value={{c.pub}}
            @onChange={{fn c.setFilter "pub"}}
          />
          <DateField
            @id="from"
            @label="Von"
            @value={{c.from}}
            @onChange={{fn c.setFilter "from"}}
          />
          <DateField
            @id="to"
            @label="Bis"
            @value={{c.to}}
            @onChange={{fn c.setFilter "to"}}
          />
        </div>
      </div>

      <div class="dis-filterbar">
        <span class="dis-result-count">
          {{if c.search.pending "Lädt…" (concat (num c.total) " Treffer")}}
          <a
            class="dis-feed-link"
            title="RSS-Feed neuer Dokumente (mit den aktiven Filtern)"
            href={{c.feedUrl}}
          >
            <Icon @name="rss" @size={{14}} />
            RSS
          </a>
        </span>
        <div class="dis-active-tags">
          {{#each c.activeFilters as |f|}}
            <div
              class="cds--tag cds--tag--teal cds--tag--sm cds--tag--filter"
              title={{f.label}}
            >
              <span class="cds--tag__label">{{f.label}}</span>
              <button
                type="button"
                class="cds--tag__close-icon"
                aria-label={{f.label}}
                {{on "click" (fn c.setFilter f.k f.clear)}}
              >
                <Icon @name="close" />
              </button>
            </div>
          {{/each}}
          {{#if c.activeFilters.length}}
            <button
              type="button"
              class="cds--btn cds--btn--ghost cds--btn--sm"
              {{on "click" c.resetAll}}
            >
              Alle zurücksetzen
            </button>
          {{/if}}
          <div class="dis-colpick">
            <ColumnPicker
              @columns={{ALL_COLUMNS}}
              @selected={{c.cols}}
              @onChange={{c.setCols}}
            />
          </div>
        </div>
      </div>

      {{#if c.search.error}}
        <p class="dis-error">Fehler: {{c.search.error}}</p>
      {{/if}}

      {{#if c.search.pending}}
        {{#unless c.results.length}}
          <TableSkeleton @columns={{this.skeletonColumns}} @rows={{10}} />
        {{/unless}}
      {{else}}
        {{#unless c.results.length}}
          <div class="dis-empty">
            Keine Dokumente gefunden — Suchbegriff anpassen oder Filter
            zurücksetzen.
          </div>
        {{/unless}}
      {{/if}}

      {{#if c.results.length}}
        <div class="cds--data-table-container">
          <div class="cds--data-table-header">
            <h4 class="cds--data-table-header__title">Dokumente</h4>
            <p class="cds--data-table-header__description">{{num c.total}}
              Treffer</p>
          </div>
          <div class="cds--data-table-content">
            <table
              class="cds--data-table cds--data-table--sm cds--data-table--zebra dis-doctable"
            >
              <thead>
                <tr>
                  {{#each c.visibleColumns as |col|}}
                    <th scope="col" class={{col.cls}}><div
                        class="cds--table-header-label"
                      >{{col.label}}</div></th>
                  {{/each}}
                </tr>
              </thead>
              <tbody>
                {{#each c.results key="id" as |d|}}
                  <tr
                    class="dis-row-clickable"
                    {{on "click" (fn this.openDoc d.id)}}
                  >
                    {{#each c.visibleColumns as |col|}}
                      <td class={{col.cls}}>
                        {{#if (eq col.key "date")}}
                          {{if d.meeting_date d.meeting_date "—"}}
                        {{else if (eq col.key "gremium")}}
                          <span class="dis-ellip" title={{d.body_name}}>{{if
                              d.body_name
                              d.body_name
                              "—"
                            }}</span>
                        {{else if (eq col.key "top")}}
                          {{#let
                            (topFull d.agenda_number d.agenda_title)
                            as |full|
                          }}
                            {{#if full}}
                              <span class="dis-ellip" title={{full}}>
                                {{#if d.agenda_number}}<span
                                    class="dis-top-num"
                                  >{{topNum d.agenda_number}}</span>{{/if}}
                                {{#if d.agenda_title}}<span
                                    class="dis-top-title"
                                  >{{d.agenda_title}}</span>{{/if}}
                              </span>
                            {{else}}
                              <span class="dis-top-none">—</span>
                            {{/if}}
                          {{/let}}
                        {{else if (eq col.key "sub")}}
                          <SubmitterChips @codes={{d.submitters}} />
                        {{else if (eq col.key "typ")}}
                          <Tag @type="gray" @size="sm">{{d.doc_type}}</Tag>
                        {{else if (eq col.key "doc")}}
                          <span class="dis-doccell">
                            <span
                              class="dis-ellip"
                              title={{d.label}}
                            >{{d.label}}</span>
                            {{#if d.snippet}}<Snippet
                                @text={{d.snippet}}
                              />{{/if}}
                          </span>
                        {{else if (eq col.key "cat")}}
                          {{#if d.category}}<Tag
                              @type="blue"
                              @size="sm"
                            >{{d.category}}</Tag>{{/if}}
                        {{else if (eq col.key "stadtteil")}}
                          {{#if d.district}}<Tag
                              @type="teal"
                              @size="sm"
                            >{{d.district}}</Tag>{{/if}}
                        {{/if}}
                      </td>
                    {{/each}}
                  </tr>
                {{/each}}
              </tbody>
            </table>
          </div>
          <Pagination
            @page={{c.page}}
            @pageSize={{PAGE_SIZE}}
            @total={{c.total}}
            @onChange={{c.setPage}}
          />
        </div>
      {{/if}}
    {{/let}}
  </template>
}
