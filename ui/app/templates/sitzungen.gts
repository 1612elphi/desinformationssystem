import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { service } from '@ember/service';
import { pageTitle } from 'ember-page-title';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import 'delphicanon/components/canon-heading.css';
import type { Meeting } from 'ui/api';
import type SitzungenController from 'ui/controllers/sitzungen';
import { MEETINGS_PAGE, VIEWS } from 'ui/controllers/sitzungen';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import CalendarAgenda, { type AgendaDay } from 'ui/components/calendar-agenda';
import CalendarMonth from 'ui/components/calendar-month';
import CalendarWeek from 'ui/components/calendar-week';
import DataTable, { type Column } from 'ui/components/data-table';
import { SelectField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import Tag from 'ui/components/tag';
import shortcut from 'ui/modifiers/shortcut';
import {
  KINDS,
  MONTH,
  RANGE,
  addDays,
  addMonths,
  bodyKind,
  iso,
  isoWeek,
  monthWeeks,
  startLabel,
  today,
  weekStart,
  type CalMeeting,
} from 'ui/utils/calendar';
import { eq, num, publicLabel } from 'ui/utils/meta';
import './sitzungen.css';

const RANGES = ['Nur kommende', 'Alle Sitzungen'];
const VIEW_LABELS = ['Monat', 'Woche', 'Liste'];

const COLUMNS: Column[] = [
  { key: 'date', label: 'Datum', width: '7rem' },
  { key: 'time', label: 'Zeit', width: '9rem' },
  { key: 'body', label: 'Gremium' },
  { key: 'public', label: 'Öffentlich', width: '9rem' },
  { key: 'agenda', label: 'TOPs', width: '4rem', cls: 'dis-num' },
  { key: 'docs', label: 'Dok.', width: '4rem', cls: 'dis-num' },
];

const STEP: Record<string, number> = {
  ArrowLeft: -1,
  ArrowRight: 1,
  ArrowUp: -7,
  ArrowDown: 7,
};

const orDash = (s?: string) => s || '—';
const publicText = (p?: number | null) => publicLabel(p).text;
const rowLabel = (m: Meeting) =>
  [m.date, m.time, m.body_name].filter(Boolean).join(' ');

// The grid re-renders its roving tab stop first; focus it on the next frame.
const focusDay = (d: Date) =>
  requestAnimationFrame(() =>
    document.querySelector<HTMLElement>(`[data-day="${iso(d)}"]`)?.focus(),
  );

interface Signature {
  Args: { model: ApplicationModel; controller: SitzungenController };
}

export default class SitzungenTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get c() {
    return this.args.controller;
  }

  get committeeItems() {
    return [
      { id: '', label: 'Alle Gremien' },
      ...(this.args.model.facets.value?.committees ?? []).map((c) => ({
        id: c.name,
        label: c.name,
      })),
    ];
  }

  get viewIndex() {
    return Math.max(0, VIEWS.indexOf(this.c.view));
  }

  get isList() {
    return this.c.view === 'liste';
  }

  get isWeek() {
    return this.c.view === 'woche';
  }

  get weeks() {
    return monthWeeks(this.c.focus);
  }

  get month() {
    return this.c.focus.getMonth();
  }

  get weekDays() {
    const s = weekStart(this.c.focus);
    return [0, 1, 2, 3, 4, 5, 6].map((i) => addDays(s, i));
  }

  get title() {
    const f = this.c.focus;
    if (!this.isWeek) return MONTH.format(f);
    const s = weekStart(f);
    return `KW ${isoWeek(f)} · ${RANGE.formatRange(s, addDays(s, 6))}`;
  }

  get loaded(): CalMeeting[] {
    return this.c.range.value?.meetings ?? [];
  }

  get filtered() {
    const k = this.c.kind;
    return k
      ? this.loaded.filter((m) => bodyKind(m.body_name) === k)
      : this.loaded;
  }

  get byDay() {
    const map = new Map<string, CalMeeting[]>();
    const sorted = [...this.filtered].sort(
      (a, b) =>
        (startLabel(a.time) ? 0 : 1) - (startLabel(b.time) ? 0 : 1) ||
        (a.time ?? '').localeCompare(b.time ?? '', 'de', { numeric: true }) ||
        (a.body_name ?? '').localeCompare(b.body_name ?? '', 'de'),
    );
    for (const m of sorted) {
      if (!m.date) continue;
      map.set(m.date, [...(map.get(m.date) ?? []), m]);
    }
    return map;
  }

  // shown days, for the month only those inside it
  get shownDays(): Date[] {
    if (this.isWeek) return this.weekDays;
    return this.weeks.flat().filter((d) => d.getMonth() === this.month);
  }

  get shownCount() {
    return this.shownDays.reduce(
      (n, d) => n + (this.byDay.get(iso(d))?.length ?? 0),
      0,
    );
  }

  get legend() {
    const keys = new Set(this.shownDays.map(iso));
    const inSpan = this.loaded.filter((m) => keys.has(m.date ?? ''));
    return KINDS.map((k) => ({
      ...k,
      n: inSpan.filter((m) => bodyKind(m.body_name) === k.id).length,
      on: this.c.kind === k.id,
    }));
  }

  get agendaDays(): AgendaDay[] {
    return this.shownDays
      .map((date) => ({ date, meetings: this.byDay.get(iso(date)) ?? [] }))
      .filter((d) => d.meetings.length);
  }

  get focusDay(): AgendaDay[] {
    return [
      { date: this.c.focus, meetings: this.byDay.get(iso(this.c.focus)) ?? [] },
    ];
  }

  get meetings() {
    return this.c.meetings.value?.meetings ?? [];
  }

  get total() {
    return this.c.meetings.value?.total ?? 0;
  }

  get paged() {
    return this.total > MEETINGS_PAGE;
  }

  get range() {
    return this.c.upcoming ? 0 : 1;
  }

  pickRange = (i: number) => this.c.setUpcoming(i === 0);
  pickView = (i: number) => this.c.setView(VIEWS[i] ?? 'monat');

  openMeeting = (m: Meeting) => this.nav.open({ meeting: m.id });

  move = (d: Date) => {
    this.c.setDay(d);
    focusDay(d);
  };

  step = (n: number) =>
    this.move(
      this.isWeek ? addDays(this.c.focus, 7 * n) : addMonths(this.c.focus, n),
    );

  prev = () => this.step(-1);
  next = () => this.step(1);
  goToday = () => this.move(today());

  enterDay = () => {
    const day = this.byDay.get(iso(this.c.focus)) ?? [];
    if (day.length === 1) this.openMeeting(day[0]!);
    else if (day.length)
      document.querySelector<HTMLElement>('.dis-cal-panel button')?.focus();
  };

  onGridKey = (e: KeyboardEvent) => {
    const f = this.c.focus;
    const by = STEP[e.key];
    if (by) this.move(addDays(f, by));
    else if (e.key === 'Home') this.move(weekStart(f));
    else if (e.key === 'End') this.move(addDays(weekStart(f), 6));
    else if (e.key === 'Enter' || e.key === ' ') this.enterDay();
    else return;
    e.preventDefault();
  };

  view = (v: (typeof VIEWS)[number]) => () => {
    this.c.setView(v);
    if (v !== 'liste') focusDay(this.c.focus);
  };

  <template>
    {{pageTitle "Sitzungen"}}
    <h1 class="dis-visually-hidden">Sitzungen</h1>
    {{#let @controller as |c|}}
      <div
        class="dis-sitzungen"
        {{shortcut "m" (this.view "monat")}}
        {{shortcut "w" (this.view "woche")}}
        {{shortcut "l" (this.view "liste")}}
      >
        <CanonRows @framed={{true}} class="dis-sitzungen-filters">
          <CanonRow @title="Ansicht" @form="stacked" as |titleId|>
            <CanonSegmented
              aria-labelledby={{titleId}}
              @items={{VIEW_LABELS}}
              @selected={{this.viewIndex}}
              @onChange={{this.pickView}}
            />
          </CanonRow>
          {{#if this.isList}}
            <CanonRow @title="Zeitraum" @form="stacked" as |titleId|>
              <CanonSegmented
                aria-labelledby={{titleId}}
                @items={{RANGES}}
                @selected={{this.range}}
                @onChange={{this.pickRange}}
              />
            </CanonRow>
          {{/if}}
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

        {{#if this.isList}}
          <section
            class="dis-sitzungen-list"
            aria-labelledby="dis-sitzungen-title"
          >
            <h2
              id="dis-sitzungen-title"
              class="canon-heading dis-sitzungen-title"
            >
              <span>{{if
                  c.upcoming
                  "Kommende Sitzungen"
                  "Alle Sitzungen"
                }}</span>
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
        {{else}}
          <section
            class="dis-cal"
            aria-labelledby="dis-cal-title"
            {{shortcut "PageUp" this.prev}}
            {{shortcut "PageDown" this.next}}
            {{shortcut "[" this.prev}}
            {{shortcut "]" this.next}}
            {{shortcut "t" this.goToday}}
          >
            <header class="dis-cal-bar">
              <div class="dis-cal-nav">
                <CanonButton
                  @kind="cell"
                  @icon={{component Icon name="chevron-left"}}
                  @label={{if this.isWeek "Vorherige Woche" "Vorheriger Monat"}}
                  @onClick={{this.prev}}
                />
                <CanonButton
                  @kind="cell"
                  @text="Heute"
                  @onClick={{this.goToday}}
                />
                <CanonButton
                  @kind="cell"
                  @icon={{component Icon name="chevron-right"}}
                  @label={{if this.isWeek "Nächste Woche" "Nächster Monat"}}
                  @onClick={{this.next}}
                />
              </div>
              <h2 id="dis-cal-title" class="dis-cal-title" aria-live="polite">
                {{this.title}}
                {{#if c.range.value}}
                  <span class="dis-sitzungen-count">{{num this.shownCount}}
                    Sitzung(en)</span>
                {{/if}}
              </h2>
              <div class="dis-cal-legend" role="group" aria-label="Art">
                {{#each this.legend as |k|}}
                  <button
                    type="button"
                    class="dis-cal-legend-item"
                    data-kind={{k.id}}
                    aria-pressed={{if k.on "true" "false"}}
                    {{on "click" (fn c.setKind k.id)}}
                  >
                    <span class="dis-cal-swatch" aria-hidden="true"></span>
                    {{k.label}}
                    <span class="dis-cal-legend-n">{{num k.n}}</span>
                  </button>
                {{/each}}
              </div>
            </header>
            <LoadState @load={{c.range}}>
              <div class="dis-cal-body" data-view={{c.view}}>
                <div class="dis-cal-main">
                  {{#if this.isWeek}}
                    <CalendarWeek
                      @days={{this.weekDays}}
                      @focus={{c.focus}}
                      @byDay={{this.byDay}}
                      @label={{this.title}}
                      @onFocus={{c.setDay}}
                      @onKeydown={{this.onGridKey}}
                      @onOpen={{this.openMeeting}}
                    />
                  {{else}}
                    <CalendarMonth
                      @weeks={{this.weeks}}
                      @month={{this.month}}
                      @focus={{c.focus}}
                      @byDay={{this.byDay}}
                      @label={{this.title}}
                      @onFocus={{c.setDay}}
                      @onKeydown={{this.onGridKey}}
                      @onOpen={{this.openMeeting}}
                    />
                  {{/if}}
                  <p class="dis-cal-keys" aria-hidden="true">
                    <kbd>←</kbd><kbd>→</kbd><kbd>↑</kbd><kbd>↓</kbd>
                    Tag ·
                    <kbd>Enter</kbd>
                    öffnen ·
                    <kbd>[</kbd><kbd>]</kbd>
                    blättern ·
                    <kbd>T</kbd>
                    heute ·
                    <kbd>M</kbd><kbd>W</kbd><kbd>L</kbd>
                    Ansicht
                  </p>
                </div>
                <CalendarAgenda
                  class="dis-cal-phone"
                  @days={{this.agendaDays}}
                  @onOpen={{this.openMeeting}}
                />
                {{#unless this.agendaDays.length}}
                  <div class="dis-cal-phone">
                    <CanonEmptyRow @text="Keine Sitzungen." />
                  </div>
                {{/unless}}
                <aside class="dis-cal-panel" aria-label="Tag">
                  <CalendarAgenda
                    @days={{this.focusDay}}
                    @onOpen={{this.openMeeting}}
                  />
                </aside>
              </div>
            </LoadState>
          </section>
        {{/if}}
      </div>
    {{/let}}
  </template>
}
