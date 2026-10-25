import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import CalendarChip from 'ui/components/calendar-chip';
import rough from 'ui/modifiers/rough';
import {
  DAY_LONG,
  WEEKDAYS,
  iso,
  isWeekend,
  type CalMeeting,
} from 'ui/utils/calendar';
import './calendar-month.css';

const SHOWN = 3;

interface Cell {
  date: Date;
  key: string;
  day: number;
  label: string;
  meetings: CalMeeting[];
  shown: CalMeeting[];
  more: number;
  outside: boolean;
  weekend: boolean;
  today: boolean;
  focused: boolean;
}

interface Signature {
  Element: HTMLDivElement;
  Args: {
    weeks: Date[][];
    month: number;
    focus: Date;
    byDay: Map<string, CalMeeting[]>;
    label: string;
    onFocus: (d: Date) => void;
    onKeydown: (e: KeyboardEvent) => void;
    onOpen: (m: CalMeeting) => void;
  };
}

// Month grid, Monday first; one tab stop, arrows move between days.
export default class CalendarMonth extends Component<Signature> {
  get rows(): Cell[][] {
    const focus = iso(this.args.focus);
    const now = iso(new Date());
    return this.args.weeks.map((week) =>
      week.map((date) => {
        const key = iso(date);
        const meetings = this.args.byDay.get(key) ?? [];
        const shown =
          meetings.length > SHOWN + 1 ? meetings.slice(0, SHOWN) : meetings;
        return {
          date,
          key,
          day: date.getDate(),
          label: `${DAY_LONG.format(date)}, ${meetings.length} Sitzung(en)`,
          meetings,
          shown,
          more: meetings.length - shown.length,
          outside: date.getMonth() !== this.args.month,
          weekend: isWeekend(date),
          today: key === now,
          focused: key === focus,
        };
      }),
    );
  }

  <template>
    <div
      class="dis-cal-month"
      role="grid"
      aria-label={{@label}}
      {{on "keydown" @onKeydown}}
      ...attributes
    >
      <div class="dis-cal-month-row dis-cal-month-head" role="row">
        {{#each WEEKDAYS as |w|}}
          <div class="dis-cal-month-wd" role="columnheader">{{w}}</div>
        {{/each}}
      </div>
      {{#each this.rows key="@index" as |week|}}
        <div class="dis-cal-month-row" role="row">
          {{#each week key="key" as |c|}}
            <div
              class="dis-cal-cell"
              role="gridcell"
              tabindex={{if c.focused "0" "-1"}}
              aria-label={{c.label}}
              aria-selected={{if c.focused "true" "false"}}
              data-day={{c.key}}
              data-outside={{if c.outside "true"}}
              data-weekend={{if c.weekend "true"}}
              data-today={{if c.today "true"}}
              {{on "focusin" (fn @onFocus c.date)}}
            >
              {{! a static branch, so moving the focus does not redraw the mark }}
              {{#if c.today}}
                <span
                  class="dis-cal-date"
                  aria-hidden="true"
                  {{rough "circle" padding=5 delay=400}}
                >{{c.day}}</span>
              {{else}}
                <span class="dis-cal-date" aria-hidden="true">{{c.day}}</span>
              {{/if}}
              {{#each c.shown as |m|}}
                <CalendarChip
                  @meeting={{m}}
                  @onOpen={{@onOpen}}
                  tabindex="-1"
                />
              {{/each}}
              {{#if c.more}}
                <span class="dis-cal-more" aria-hidden="true">+{{c.more}}</span>
              {{/if}}
            </div>
          {{/each}}
        </div>
      {{/each}}
    </div>
  </template>
}
