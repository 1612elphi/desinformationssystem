import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import { SEAT_ORDER, SUBMITTERS, VOTE_LABEL } from 'ui/utils/meta';
import { CHAMBER, CHAMBER_COLS, DAIS } from 'ui/utils/chamber';
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
  absent: boolean;
}

interface Desk {
  x: number;
  y: number;
  label: string;
  placed?: Placed;
}

// viewBox units
const DESK_W = 1;
const DESK_H = 0.62;
const GAP = 0.14;
const DAIS_GAP = 0.6;
const chamberIds = new Set(
  [...CHAMBER, ...DAIS].map((s) => s.person_id).filter(Boolean),
);

const VOTE_ORDER = ['ja', 'nein', 'enthaltung', 'abwesend'];
const rank = (xs: string[], x?: string | null) => {
  const i = xs.indexOf(x ?? '');
  return i < 0 ? xs.length : i;
};

interface Signature {
  Element: SVGSVGElement;
  Args: {
    seats: Seat[];
    mode?: 'vote' | 'party';
    label: string;
    onPick?: (personId: string) => void;
  };
}

export default class SeatChart extends Component<Signature> {
  // most person_ids on plan
  get isHall() {
    const known = this.args.seats.filter((s) => s.person_id);
    const onPlan = known.filter((s) => chamberIds.has(s.person_id!));
    return known.length > 0 && onPlan.length >= known.length * 0.75;
  }

  get placed(): Placed[] {
    const seats = [...this.args.seats].sort(
      (a, b) =>
        rank(SEAT_ORDER, a.party_code) - rank(SEAT_ORDER, b.party_code) ||
        rank(VOTE_ORDER, a.vote) - rank(VOTE_ORDER, b.vote),
    );
    const pos = arc(seats.length);
    return seats.map((seat, i) => this.paint(seat, pos[i]!.x, pos[i]!.y));
  }

  get desks(): Desk[] {
    const byId = new Map(
      this.args.seats.filter((s) => s.person_id).map((s) => [s.person_id!, s]),
    );
    const used = new Set<Seat>();
    const desk = (x: number, y: number, label: string, seat?: Seat) => {
      if (seat) used.add(seat);
      return { x, y, label, placed: seat && this.paint(seat, x, y) };
    };
    const at = (id: string | null) => (id ? byId.get(id) : undefined);
    const step = DESK_W + GAP;
    const daisLeft = ((CHAMBER_COLS - DAIS.length) * step) / 2;
    const out = DAIS.map((d, i) =>
      desk(daisLeft + i * step, 0, `${d.role} ${d.name}`, at(d.person_id)),
    );
    const top = DESK_H + DAIS_GAP;
    for (const c of CHAMBER)
      out.push(
        desk(
          c.col * step,
          top + c.row * (DESK_H + GAP),
          c.name,
          at(c.person_id),
        ),
      );
    const rest = this.args.seats.filter((s) => !used.has(s));
    const extra =
      top + (Math.max(...CHAMBER.map((c) => c.row)) + 1) * (DESK_H + GAP) + GAP;
    rest.forEach((s, i) =>
      out.push(
        desk(
          (i % CHAMBER_COLS) * step,
          extra + Math.floor(i / CHAMBER_COLS) * (DESK_H + GAP),
          s.name,
          s,
        ),
      ),
    );
    return out;
  }

  get hallBox() {
    const ys = this.desks.map((d) => d.y);
    const w = CHAMBER_COLS * (DESK_W + GAP) - GAP;
    const h = Math.max(...ys) + DESK_H;
    return `-0.05 -0.05 ${w + 0.1} ${h + 0.1}`;
  }

  paint(seat: Seat, x: number, y: number): Placed {
    const byParty = this.args.mode === 'party';
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
    const title = [
      seat.name,
      seat.party_code,
      !byParty && (VOTE_LABEL[vote] ?? vote),
    ]
      .filter(Boolean)
      .join(' · ');
    return { seat, x, y, fill, title, absent: !byParty && !seat.vote };
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
    {{#if this.isHall}}
      <svg
        class="dis-seats dis-seats--hall {{if @onPick 'dis-seats--pickable'}}"
        viewBox={{this.hallBox}}
        role="img"
        aria-label={{@label}}
        ...attributes
      >
        {{#each this.desks as |d|}}
          {{#if d.placed}}
            <rect
              class="dis-seat {{if d.placed.absent 'dis-seat--absent'}}"
              x={{d.x}}
              y={{d.y}}
              width={{DESK_W}}
              height={{DESK_H}}
              style={{this.fillStyle d.placed.fill}}
              {{on "click" (fn this.pick d.placed.seat)}}
            ><title>{{d.placed.title}}</title></rect>
          {{else}}
            <rect
              class="dis-seat dis-seat--empty"
              x={{d.x}}
              y={{d.y}}
              width={{DESK_W}}
              height={{DESK_H}}
            ><title>{{d.label}}</title></rect>
          {{/if}}
        {{/each}}
      </svg>
    {{else}}
      <svg
        class="dis-seats {{if @onPick 'dis-seats--pickable'}}"
        viewBox="-1.08 -1.08 2.16 1.16"
        role="img"
        aria-label={{@label}}
        ...attributes
      >
        {{#each this.placed as |p|}}
          <circle
            class="dis-seat {{if p.absent 'dis-seat--absent'}}"
            cx={{p.x}}
            cy={{p.y}}
            r={{this.dot}}
            style={{this.fillStyle p.fill}}
            {{on "click" (fn this.pick p.seat)}}
          ><title>{{p.title}}</title></circle>
        {{/each}}
      </svg>
    {{/if}}
  </template>
}
