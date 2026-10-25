import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import { getMeetings } from 'ui/api';
import Load from 'ui/utils/load';
import {
  addDays,
  iso,
  monthWeeks,
  parseIso,
  weekStart,
  type CalMeeting,
} from 'ui/utils/calendar';

export const MEETINGS_PAGE = 100;

export type View = 'monat' | 'woche' | 'liste';
export const VIEWS: View[] = ['monat', 'woche', 'liste'];

type Range = { meetings: CalMeeting[]; total: number };

export default class SitzungenController extends Controller {
  queryParams = ['view', 'day', 'kind', 'upcoming', 'committee', 'page'];

  @tracked view: View = 'monat';
  // iso; '' is today
  @tracked day = '';
  @tracked kind = '';
  @tracked upcoming = true;
  @tracked committee = '';
  @tracked page = 1;

  get focus() {
    return parseIso(this.day);
  }

  setView = (v: View) => (this.view = v);
  setDay = (d: Date) => (this.day = iso(d));
  setKind = (k: string) => (this.kind = this.kind === k ? '' : k);

  setUpcoming = (v: boolean) => {
    this.upcoming = v;
    this.page = 1;
  };

  setCommittee = (v: string) => {
    this.committee = v;
    this.page = 1;
  };

  setPage = (p: number) => (this.page = p);

  get span(): [Date, Date] {
    if (this.view === 'woche') {
      const s = weekStart(this.focus);
      return [s, addDays(s, 6)];
    }
    const weeks = monthWeeks(this.focus);
    return [weeks[0]![0]!, weeks.at(-1)![6]!];
  }

  private memo: { key: string; load?: Load<Range> } = { key: '' };

  // in-span moves keep data
  get range(): Load<Range> {
    const [from, to] = this.span.map(iso) as [string, string];
    const key = [from, to, this.committee].join('|');
    const memo = this.memo;
    if (key !== memo.key || !memo.load) {
      memo.key = key;
      memo.load = new Load(
        getMeetings({ from, to, committee: this.committee, limit: 500 }),
        memo.load,
      );
    }
    return memo.load;
  }

  @cached
  get meetings() {
    return new Load(
      getMeetings({
        upcoming: this.upcoming,
        committee: this.committee,
        limit: MEETINGS_PAGE,
        offset: (this.page - 1) * MEETINGS_PAGE,
      }),
    );
  }

  get icsUrl() {
    return `/api/meetings.ics${this.committee ? `?committee=${encodeURIComponent(this.committee)}` : ''}`;
  }
}
