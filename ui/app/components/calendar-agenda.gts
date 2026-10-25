import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import Icon from 'ui/components/icon';
import {
  DAY_LONG,
  bodyKind,
  iso,
  isLive,
  type CalMeeting,
} from 'ui/utils/calendar';
import { num } from 'ui/utils/meta';
import './calendar-agenda.css';

export interface AgendaDay {
  date: Date;
  meetings: CalMeeting[];
}

interface Signature {
  Element: HTMLDivElement;
  Args: {
    days: AgendaDay[];
    onOpen: (m: CalMeeting) => void;
  };
}

// Meetings grouped by day, one button per meeting.
export default class CalendarAgenda extends Component<Signature> {
  get days() {
    const now = iso(new Date());
    return this.args.days.map((d) => ({
      key: iso(d.date),
      title: DAY_LONG.format(d.date),
      today: iso(d.date) === now,
      meetings: d.meetings.map((m) => ({
        m,
        kind: bodyKind(m.body_name),
        live: isLive(m),
      })),
    }));
  }

  <template>
    <div class="dis-cal-agenda" ...attributes>
      {{#each this.days as |d|}}
        <section class="dis-cal-agenda-day" data-today={{if d.today "true"}}>
          <h3 class="dis-cal-agenda-title">{{d.title}}</h3>
          {{#each d.meetings as |x|}}
            <button
              type="button"
              class="dis-cal-agenda-item"
              data-kind={{x.kind}}
              {{on "click" (fn @onOpen x.m)}}
            >
              <span class="dis-cal-agenda-time">
                {{#if x.live}}<span class="dis-cal-pulse"></span> live ·{{/if}}
                {{if x.m.time x.m.time "—"}}
              </span>
              <span class="dis-cal-agenda-body">{{if
                  x.m.body_name
                  x.m.body_name
                  "—"
                }}</span>
              {{#if x.m.location}}
                <span class="dis-cal-agenda-loc">{{x.m.location}}</span>
              {{/if}}
              <span class="dis-cal-agenda-counts">
                {{#if x.m.agenda_count}}
                  <span>{{num x.m.agenda_count}} TOPs</span>
                {{/if}}
                {{#if x.m.documents}}
                  <span><Icon @name="document" @size={{12}} />
                    {{num x.m.documents}}
                    Dok.</span>
                {{/if}}
                {{#if x.m.vote_count}}
                  <span><Icon @name="checkmark" @size={{12}} />
                    {{num x.m.vote_count}}
                    Abst.</span>
                {{/if}}
              </span>
            </button>
          {{else}}
            <CanonEmptyRow @text="Keine Sitzungen." />
          {{/each}}
        </section>
      {{/each}}
    </div>
  </template>
}
