import Component from '@glimmer/component';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { modifier } from 'ember-modifier';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import type { Meeting } from 'ui/api';
import DocsGlyph from 'ui/components/docs-glyph';
import { LoadState } from 'ui/components/loading';
import Icon from 'ui/components/icon';
import Tag from 'ui/components/tag';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import { getRecent, getWeek, isoDay, relDate } from 'ui/utils/api-dokumente';
import Load from 'ui/utils/load';
import './docs-week.css';

const weekday = new Intl.DateTimeFormat('de-DE', { weekday: 'short' });
const dayMonth = new Intl.DateTimeFormat('de-DE', {
  day: 'numeric',
  month: 'numeric',
});
const longDate = new Intl.DateTimeFormat('de-DE', {
  weekday: 'long',
  day: 'numeric',
  month: 'long',
});

const parse = (iso: string) => {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y ?? 0, (m ?? 1) - 1, d ?? 1);
};

// ISO 8601 week number: the week with the year's first Thursday is week 1.
function isoWeek(iso: string) {
  const d = parse(iso);
  d.setDate(d.getDate() + 3 - ((d.getDay() + 6) % 7));
  const jan4 = new Date(d.getFullYear(), 0, 4);
  return (
    1 +
    Math.round(
      ((d.getTime() - jan4.getTime()) / 864e5 - 3 + ((jan4.getDay() + 6) % 7)) /
        7,
    )
  );
}

// scroll today's card into the strip when the week starts off-screen
const reveal = modifier((el: HTMLElement, [on]: [boolean]) => {
  const strip = el.parentElement;
  if (!on || !strip) return;
  strip.scrollLeft +=
    el.getBoundingClientRect().left - strip.getBoundingClientRect().left;
});

const startTime = (t?: string) => (t ? t.split(' bis ')[0] : '');
const docTitle = (d: { agenda_title?: string; label: string }) =>
  d.agenda_title || d.label;

interface Day {
  iso: string;
  weekday: string;
  date: string;
  label: string;
  today: boolean;
  meetings: (Meeting & { live: boolean })[];
}

// The front page band: this week's sessions and the newest documents.
export default class DocsWeek extends Component {
  @service declare nav: NavService;

  today = new Date();
  week = new Load(getWeek(this.today));
  recent = new Load(getRecent(12));

  get kw() {
    const w = this.week.value;
    return w ? `KW ${isoWeek(w.days[0]!)}` : '';
  }

  get days(): Day[] {
    const w = this.week.value;
    if (!w) return [];
    const todayIso = isoDay(this.today);
    return w.days.map((iso) => {
      const d = parse(iso);
      return {
        iso,
        weekday: weekday.format(d),
        date: dayMonth.format(d),
        label: longDate.format(d),
        today: iso === todayIso,
        meetings: w.meetings
          .filter((m) => m.date === iso)
          .map((m) => ({ ...m, live: w.live.includes(m.id) })),
      };
    });
  }

  openMeeting = (id: string) => this.nav.open({ meeting: id });
  openDoc = (id: string) => this.nav.open({ doc: id });

  <template>
    <section class="dis-week" aria-labelledby="dis-week-title">
      <header class="dis-week-head">
        <h2 id="dis-week-title"><Icon @name="docs-day" @size={{20}} />Diese
          Woche</h2>
        <span class="dis-week-kw">{{this.kw}}</span>
      </header>

      <LoadState @load={{this.week}}>
        <ol class="dis-week-days" aria-label="Sitzungen">
          {{#each this.days key="iso" as |d|}}
            <li
              class="dis-day
                {{if d.today 'is-today'}}
                {{unless d.meetings.length 'is-empty'}}"
              aria-current={{if d.today "date"}}
              {{reveal d.today}}
            >
              <span class="dis-day-head" title={{d.label}}>
                <span class="dis-day-wd">{{d.weekday}}</span>
                <span class="dis-day-date">{{d.date}}</span>
                {{#if d.today}}<span
                    class="dis-day-today"
                    {{rough "circle" padding=3 delay=600}}
                  >Heute</span>{{/if}}
              </span>
              {{#each d.meetings key="id" as |m|}}
                <button
                  type="button"
                  class="dis-day-meeting"
                  {{on "click" (fn this.openMeeting m.id)}}
                >
                  {{#if m.live}}
                    <span class="dis-day-live" aria-label="Live"></span>
                  {{/if}}
                  <span
                    class="dis-day-body"
                    {{rough
                      "circle"
                      color="--canon-destructive"
                      padding=3
                      show=m.live
                      delay=900
                    }}
                  >{{m.body_name}}</span>
                  {{#if m.time}}
                    <span class="dis-day-time"><Icon
                        @name="time"
                        @size={{12}}
                      />{{startTime m.time}}</span>
                  {{/if}}
                </button>
              {{/each}}
            </li>
          {{/each}}
        </ol>
      </LoadState>

      <div class="dis-week-section">
        <h3><Icon @name="docs-new" @size={{14}} />Neue Dokumente</h3>
        <LoadState @load={{this.recent}}>
          {{#if this.recent.value.length}}
            <ul class="dis-week-strip">
              {{#each this.recent.value key="id" as |d|}}
                <li>
                  <button
                    type="button"
                    class="dis-wdoc"
                    {{on "click" (fn this.openDoc d.id)}}
                  >
                    <span class="dis-wdoc-head">
                      <DocsGlyph @type={{d.doc_type}} />
                      <span class="dis-wdoc-type">{{d.doc_type}}</span>
                      {{#if d.meeting_date}}
                        <time
                          class="dis-wdoc-date"
                          datetime={{d.meeting_date}}
                          title={{d.meeting_date}}
                        >{{relDate d.meeting_date this.today}}</time>
                      {{/if}}
                    </span>
                    <span class="dis-wdoc-body">{{d.body_name}}</span>
                    <span class="dis-wdoc-title">{{docTitle d}}</span>
                    {{#if d.category}}<Tag>{{d.category}}</Tag>{{/if}}
                  </button>
                </li>
              {{/each}}
            </ul>
          {{else}}
            <CanonEmptyRow @text="Keine Dokumente" />
          {{/if}}
        </LoadState>
      </div>

    </section>
  </template>
}
