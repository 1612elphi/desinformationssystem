import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import CalendarChip from 'ui/components/calendar-chip';
import rough from 'ui/modifiers/rough';
import {
  DAY_LONG,
  DAY_SHORT,
  clock,
  iso,
  isWeekend,
  lanes,
  timeWindow,
  type CalMeeting,
} from 'ui/utils/calendar';
import './calendar-week.css';

const win = (m: CalMeeting) => timeWindow(m.time)!;

interface Signature {
  Element: HTMLDivElement;
  Args: {
    days: Date[];
    focus: Date;
    byDay: Map<string, CalMeeting[]>;
    label: string;
    onFocus: (d: Date) => void;
    onKeydown: (e: KeyboardEvent) => void;
    onOpen: (m: CalMeeting) => void;
  };
}

// Week as a time grid: meetings placed by their window, untimed ones on top.
export default class CalendarWeek extends Component<Signature> {
  get all() {
    return this.args.days.flatMap((d) => this.args.byDay.get(iso(d)) ?? []);
  }

  // whole hours the grid shows
  get hours(): [number, number] {
    const ws = this.all
      .map((m) => timeWindow(m.time))
      .filter((w) => w !== null);
    const from = Math.min(15, ...ws.map((w) => Math.floor(w.start / 60)));
    const to = Math.max(21, ...ws.map((w) => Math.ceil(w.end / 60)));
    return [from, Math.min(24, to)];
  }

  get ticks() {
    const [from, to] = this.hours;
    return Array.from({ length: to - from }, (_, i) => clock((from + i) * 60));
  }

  get gridStyle() {
    const [from, to] = this.hours;
    return htmlSafe(`--dis-cal-hours: ${to - from}`);
  }

  get columns() {
    const focus = iso(this.args.focus);
    const now = iso(new Date());
    const [from, to] = this.hours;
    const span = (to - from) * 60;
    return this.args.days.map((date) => {
      const key = iso(date);
      const meetings = this.args.byDay.get(key) ?? [];
      const timed = meetings.filter((m) => timeWindow(m.time));
      return {
        date,
        key,
        head: DAY_SHORT.format(date).replace('.,', ''),
        label: `${DAY_LONG.format(date)}, ${meetings.length} Sitzung(en)`,
        untimed: meetings.filter((m) => !timeWindow(m.time)),
        placed: lanes(timed, win).map(({ item, lane, lanes: n }) => {
          const w = win(item);
          const top = ((w.start - from * 60) / span) * 100;
          const height = ((w.end - w.start) / span) * 100;
          return {
            m: item,
            style: htmlSafe(
              `inset-block-start: ${top}%; block-size: ${height}%; ` +
                `inset-inline-start: ${(lane / n) * 100}%; inline-size: ${100 / n}%`,
            ),
          };
        }),
        weekend: isWeekend(date),
        today: key === now,
        focused: key === focus,
      };
    });
  }

  <template>
    <div
      class="dis-cal-week"
      role="grid"
      aria-label={{@label}}
      style={{this.gridStyle}}
      {{on "keydown" @onKeydown}}
      ...attributes
    >
      <div class="dis-cal-week-row" role="row">
        <div class="dis-cal-week-gutter" aria-hidden="true">
          <span></span>
          <span></span>
          <ol class="dis-cal-week-ticks">
            {{#each this.ticks as |t|}}
              <li>{{t}}</li>
            {{/each}}
          </ol>
        </div>
        {{#each this.columns key="key" as |c|}}
          <div
            class="dis-cal-week-day"
            role="gridcell"
            tabindex={{if c.focused "0" "-1"}}
            aria-label={{c.label}}
            aria-selected={{if c.focused "true" "false"}}
            data-day={{c.key}}
            data-weekend={{if c.weekend "true"}}
            data-today={{if c.today "true"}}
            {{on "focusin" (fn @onFocus c.date)}}
          >
            {{! a static branch, so moving the focus does not redraw the mark }}
            {{#if c.today}}
              <span
                class="dis-cal-week-head"
                aria-hidden="true"
                {{rough
                  "underline"
                  color="--dis-highlight"
                  strokeWidth=3
                  padding=0
                  delay=400
                }}
              >{{c.head}}</span>
            {{else}}
              <span
                class="dis-cal-week-head"
                aria-hidden="true"
              >{{c.head}}</span>
            {{/if}}
            <div class="dis-cal-week-untimed">
              {{#each c.untimed as |m|}}
                <CalendarChip
                  @meeting={{m}}
                  @onOpen={{@onOpen}}
                  tabindex="-1"
                />
              {{/each}}
            </div>
            <div class="dis-cal-week-track">
              {{#each c.placed as |p|}}
                <CalendarChip
                  @meeting={{p.m}}
                  @onOpen={{@onOpen}}
                  @full={{true}}
                  class="dis-cal-week-slot"
                  style={{p.style}}
                  tabindex="-1"
                />
              {{/each}}
            </div>
          </div>
        {{/each}}
      </div>
    </div>
  </template>
}
