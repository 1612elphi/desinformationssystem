import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import { SEAT_ORDER, SUBMITTERS, VOTE_LABEL } from 'ui/utils/meta';
import { arc } from 'ui/utils/seats';
import './seat-chart.css';

export interface Seat {
  name: string;
  party_code?: string | null;
  person_id?: string | null;
  vote?: string | null;
}

interface Placed {
  seat: Seat;
  x: number;
  y: number;
  fill: string;
  title: string;
}

const VOTE_ORDER = ['ja', 'nein', 'enthaltung', 'abwesend'];
const rank = (xs: string[], x?: string | null) => {
  const i = xs.indexOf(x ?? '');
  return i < 0 ? xs.length : i;
};

interface Signature {
  Element: SVGSVGElement;
  Args: {
    seats: Seat[];
    // vote: seats coloured by vote; party: by party colour
    mode?: 'vote' | 'party';
    label: string;
    onPick?: (personId: string) => void;
  };
}

// Chamber seat chart: one dot per member, grouped by party left to right.
export default class SeatChart extends Component<Signature> {
  get placed(): Placed[] {
    const seats = [...this.args.seats].sort(
      (a, b) =>
        rank(SEAT_ORDER, a.party_code) - rank(SEAT_ORDER, b.party_code) ||
        rank(VOTE_ORDER, a.vote) - rank(VOTE_ORDER, b.vote),
    );
    const pos = arc(seats.length);
    const byParty = this.args.mode === 'party';
    return seats.map((seat, i) => {
      const vote = seat.vote || 'abwesend';
      const mark = seat.party_code
        ? SUBMITTERS[seat.party_code]?.mark
        : undefined;
      const fill = byParty
        ? (mark ?? 'var(--canon-muted-foreground)')
        : vote === 'ja'
          ? 'var(--dis-yes)'
          : vote === 'nein'
            ? 'var(--dis-no)'
            : vote === 'enthaltung'
              ? 'var(--dis-abstain)'
              : 'transparent';
      const title = [seat.name, seat.party_code, VOTE_LABEL[vote] ?? vote]
        .filter(Boolean)
        .join(' · ');
      return { seat, ...pos[i]!, fill, title };
    });
  }

  get dot() {
    const n = Math.max(1, this.args.seats.length);
    return Math.min(0.06, 0.9 / Math.sqrt(n) / 2.4);
  }

  fillStyle = (fill: string) => htmlSafe(`fill: ${fill}`);

  pick = (seat: Seat) => {
    if (seat.person_id) this.args.onPick?.(seat.person_id);
  };

  <template>
    <svg
      class="dis-seats {{if @onPick 'dis-seats--pickable'}}"
      viewBox="-1.08 -1.08 2.16 1.16"
      role="img"
      aria-label={{@label}}
      ...attributes
    >
      {{#each this.placed as |p|}}
        <circle
          class="dis-seat {{unless p.seat.vote 'dis-seat--absent'}}"
          cx={{p.x}}
          cy={{p.y}}
          r={{this.dot}}
          style={{this.fillStyle p.fill}}
          {{on "click" (fn this.pick p.seat)}}
        ><title>{{p.title}}</title></circle>
      {{/each}}
    </svg>
  </template>
}
