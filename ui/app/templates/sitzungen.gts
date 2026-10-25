import Component from '@glimmer/component';
import { service } from '@ember/service';
import { pageTitle } from 'ember-page-title';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import 'delphicanon/components/canon-heading.css';
import type { Meeting } from 'ui/api';
import type SitzungenController from 'ui/controllers/sitzungen';
import { MEETINGS_PAGE } from 'ui/controllers/sitzungen';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import DataTable, { type Column } from 'ui/components/data-table';
import { SelectField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import Tag from 'ui/components/tag';
import { num, publicLabel } from 'ui/utils/meta';
import './sitzungen.css';

const RANGES = ['Nur kommende', 'Alle Sitzungen'];

const COLUMNS: Column[] = [
  { key: 'date', label: 'Datum', width: '7rem' },
  { key: 'time', label: 'Zeit', width: '9rem' },
  { key: 'body', label: 'Gremium' },
  { key: 'public', label: 'Öffentlich', width: '9rem' },
  { key: 'agenda', label: 'TOPs', width: '4rem', cls: 'dis-num' },
  { key: 'docs', label: 'Dok.', width: '4rem', cls: 'dis-num' },
];

const eq = (a: string, b: string) => a === b;
const orDash = (s?: string) => s || '—';
const publicText = (p?: number | null) => publicLabel(p).text;
const rowLabel = (m: Meeting) =>
  [m.date, m.time, m.body_name].filter(Boolean).join(' ');

interface Signature {
  Args: { model: ApplicationModel; controller: SitzungenController };
}

export default class SitzungenTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get committeeItems() {
    return [
      { id: '', label: 'Alle Gremien' },
      ...(this.args.model.facets.value?.committees ?? []).map((c) => ({
        id: c.name,
        label: c.name,
      })),
    ];
  }

  get meetings() {
    return this.args.controller.meetings.value?.meetings ?? [];
  }

  get total() {
    return this.args.controller.meetings.value?.total ?? 0;
  }

  get paged() {
    return this.total > MEETINGS_PAGE;
  }

  get range() {
    return this.args.controller.upcoming ? 0 : 1;
  }

  pickRange = (i: number) => this.args.controller.setUpcoming(i === 0);

  openMeeting = (m: Meeting) => this.nav.open({ meeting: m.id });

  <template>
    {{pageTitle "Sitzungen"}}
    <h1 class="dis-visually-hidden">Sitzungen</h1>
    {{#let @controller as |c|}}
      <div class="dis-sitzungen">
        <CanonRows @framed={{true}} class="dis-sitzungen-filters">
          <CanonRow @title="Zeitraum" @form="stacked" as |titleId|>
            <CanonSegmented
              aria-labelledby={{titleId}}
              @items={{RANGES}}
              @selected={{this.range}}
              @onChange={{this.pickRange}}
            />
          </CanonRow>
          <SelectField
            @label="Gremium"
            @items={{this.committeeItems}}
            @value={{c.committee}}
            @onChange={{c.setCommittee}}
          />
          <div class="dis-sitzungen-ics">
            <a
              href={{c.icsUrl}}
              title="Sitzungen als Kalender abonnieren (ICS)"
            >
              <Icon @name="calendar" @size={{14}} />
              Kalender abonnieren (ICS)
            </a>
          </div>
        </CanonRows>

        <section
          class="dis-sitzungen-list"
          aria-labelledby="dis-sitzungen-title"
        >
          <h2
            id="dis-sitzungen-title"
            class="canon-heading dis-sitzungen-title"
          >
            <span>{{if c.upcoming "Kommende Sitzungen" "Alle Sitzungen"}}</span>
            {{#if c.meetings.value}}
              <span class="dis-sitzungen-count">{{num this.total}}
                Sitzung(en)</span>
            {{/if}}
          </h2>
          <LoadState @load={{c.meetings}}>
            {{#if this.meetings.length}}
              <DataTable
                @columns={{COLUMNS}}
                @rows={{this.meetings}}
                @onOpen={{this.openMeeting}}
                @rowLabel={{rowLabel}}
                as |m col|
              >
                {{#if (eq col.key "date")}}
                  {{orDash m.date}}
                {{else if (eq col.key "time")}}
                  {{orDash m.time}}
                {{else if (eq col.key "body")}}
                  {{orDash m.body_name}}
                {{else if (eq col.key "public")}}
                  <Tag>{{publicText m.public}}</Tag>
                {{else if (eq col.key "agenda")}}
                  {{num m.agenda_count}}
                {{else}}
                  {{num m.documents}}
                {{/if}}
              </DataTable>
              {{#if this.paged}}
                <Pagination
                  @page={{c.page}}
                  @pageSize={{MEETINGS_PAGE}}
                  @total={{this.total}}
                  @onChange={{c.setPage}}
                />
              {{/if}}
            {{else}}
              <CanonEmptyRow
                @text={{if
                  c.upcoming
                  "Keine kommenden Sitzungen."
                  "Keine Sitzungen."
                }}
              />
            {{/if}}
          </LoadState>
        </section>
      </div>
    {{/let}}
  </template>
}
