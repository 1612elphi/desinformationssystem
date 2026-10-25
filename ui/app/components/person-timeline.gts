import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { againstParty, deDate, type PartyVote } from 'ui/utils/api-menschen';
import { VOTE_LABEL, num } from 'ui/utils/meta';
import './person-timeline.css';

interface Meet {
  id: string;
  date: string;
  label: string;
  votes: PartyVote[];
}

const dotClass = (v: PartyVote) =>
  `dis-tl-dot dis-tl-dot--${v.vote.replace(/[^a-z]/g, '') || 'abwesend'}${
    againstParty(v) ? ' dis-tl-dot--against' : ''
  }`;

interface Signature {
  Args: { votes: PartyVote[]; onOpen: (meetingId: string) => void };
}

// One block per meeting, oldest first, one square per vote coloured by the vote;
// squares against the own party are ringed.
export default class PersonTimeline extends Component<Signature> {
  @cached
  get years() {
    const byMeeting = new Map<string, Meet>();
    for (const v of this.args.votes) {
      let m = byMeeting.get(v.meeting_id);
      if (!m) {
        m = {
          id: v.meeting_id,
          date: v.meeting_date ?? '',
          label: '',
          votes: [],
        };
        byMeeting.set(v.meeting_id, m);
      }
      m.votes.push(v);
    }
    const meets = [...byMeeting.values()].sort((a, b) =>
      a.date.localeCompare(b.date),
    );
    const years: { year: string; meetings: Meet[] }[] = [];
    for (const m of meets) {
      const against = m.votes.filter(againstParty).length;
      m.label = [
        deDate(m.date),
        m.votes[0]?.body_name,
        countLabel(m.votes),
        against ? `${num(against)} ≠ Fraktion` : '',
      ]
        .filter(Boolean)
        .join(' · ');
      const y = m.date.slice(0, 4) || '—';
      if (years.at(-1)?.year !== y) years.push({ year: y, meetings: [] });
      years.at(-1)!.meetings.push(m);
    }
    return years;
  }

  <template>
    <div class="dis-tl">
      {{#each this.years key="year" as |y|}}
        <div class="dis-tl-year">
          <span class="dis-tl-label">{{y.year}}</span>
          <div class="dis-tl-meetings">
            {{#each y.meetings key="id" as |m|}}
              <button
                type="button"
                class="dis-tl-meeting"
                title={{m.label}}
                aria-label={{m.label}}
                {{on "click" (fn @onOpen m.id)}}
              >
                {{#each m.votes as |v|}}
                  <span class={{dotClass v}}></span>
                {{/each}}
              </button>
            {{/each}}
          </div>
        </div>
      {{/each}}
    </div>
  </template>
}

function countLabel(votes: PartyVote[]) {
  const c: Record<string, number> = {};
  for (const v of votes) c[v.vote] = (c[v.vote] ?? 0) + 1;
  return Object.entries(c)
    .map(([k, n]) => `${VOTE_LABEL[k] ?? k} ${num(n)}`)
    .join(', ');
}
