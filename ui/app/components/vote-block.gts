import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import type { Vote } from 'ui/api';
import Icon from 'ui/components/icon';
import SeatChart, { type Seat } from 'ui/components/seat-chart';
import Tag from 'ui/components/tag';
import VoteBar from 'ui/components/vote-bar';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import { partyName } from 'ui/utils/api-menschen';
import {
  SEAT_ORDER,
  SUBMITTERS,
  VOTE_LABEL,
  hasTally,
  num,
} from 'ui/utils/meta';
import './vote-block.css';

export interface Outcome {
  key: 'einstimmig' | 'angenommen' | 'abgelehnt';
  label: string;
  contested: boolean;
  close: boolean;
}

// margin of 49 seats
const CLOSE = 3;

export function outcome(v?: Vote): Outcome | undefined {
  if (!v || !hasTally(v)) return undefined;
  const ja = v.ja ?? 0;
  const nein = v.nein ?? 0;
  const contested = nein > 0;
  const close = contested && Math.abs(ja - nein) <= CLOSE;
  if (!contested && !(v.enthaltung ?? 0))
    return { key: 'einstimmig', label: 'Einstimmig', contested, close };
  return ja > nein
    ? { key: 'angenommen', label: 'Angenommen', contested, close }
    : { key: 'abgelehnt', label: 'Abgelehnt', contested, close };
}

const CAST = ['ja', 'nein', 'enthaltung'];
const MODES = ['Stimme', 'Partei'];

interface Party {
  code: string;
  mark?: string;
  name: string;
  n: Record<string, number>;
  line?: string;
  dissent: Seat[];
  split: boolean;
}

const rank = (code: string) => {
  const i = SEAT_ORDER.indexOf(code);
  return i < 0 ? SEAT_ORDER.length : i;
};

export function parties(seats: Seat[]): Party[] {
  const groups = new Map<string, Seat[]>();
  for (const s of seats) {
    const code = s.party_code ?? '';
    groups.set(code, [...(groups.get(code) ?? []), s]);
  }
  return [...groups]
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
    .map(([code, members]) => {
      const n: Record<string, number> = {};
      for (const m of members) {
        const v = m.vote || 'abwesend';
        n[v] = (n[v] ?? 0) + 1;
      }
      const top = Math.max(0, ...CAST.map((v) => n[v] ?? 0));
      const leaders = CAST.filter((v) => (n[v] ?? 0) === top);
      // tie: no party line
      const line = top > 0 && leaders.length === 1 ? leaders[0] : undefined;
      const dissent = line
        ? members.filter(
            (m) => m.vote && CAST.includes(m.vote) && m.vote !== line,
          )
        : [];
      return {
        code,
        mark: SUBMITTERS[code]?.mark,
        name: partyName(code),
        n,
        line,
        dissent,
        split: dissent.length > 0,
      };
    });
}

const OUTCOME_ICON: Record<string, string> = {
  einstimmig: 'check-check',
  angenommen: 'thumbs-up',
  abgelehnt: 'thumbs-down',
};
export const outcomeIcon = (key: string) => OUTCOME_ICON[key] ?? 'vote';

const voteLabel = (v?: string | null) => VOTE_LABEL[v ?? ''] ?? v ?? '';
const several = (p: Party) =>
  CAST.reduce((a, v) => a + (p.n[v] ?? 0), 0) > 1 && !!p.line;
const isHttp = (u?: string) => !!u?.startsWith('http');

interface Signature {
  Args: { vote: Vote };
}

export default class VoteBlock extends Component<Signature> {
  @service declare nav: NavService;
  @tracked mode = 0;

  get outcome() {
    return outcome(this.args.vote);
  }

  get seats(): Seat[] {
    return this.args.vote.members ?? [];
  }

  get byParty() {
    return this.seats.some((s) => s.party_code) ? parties(this.seats) : [];
  }

  get dissenters() {
    return this.byParty.reduce((a, p) => a + p.dissent.length, 0);
  }

  get absent() {
    return this.seats.filter((s) => !s.vote || s.vote === 'abwesend').length;
  }

  get chartMode() {
    return this.mode === 1 ? 'party' : 'vote';
  }

  setMode = (i: number) => (this.mode = i);

  pick = (personId: string) => this.nav.open({ person: personId });

  <template>
    <div class="dis-vote {{if this.outcome.contested 'dis-vote--contested'}}">
      <div class="dis-vote-head">
        {{#if this.outcome}}
          <span
            class="dis-outcome dis-outcome--{{this.outcome.key}}
              {{if this.outcome.contested 'dis-outcome--contested'}}"
            title={{if this.outcome.close "Knapp"}}
            {{rough
              (if this.outcome.close "box" "underline")
              color="--dis-no"
              show=this.outcome.contested
              padding=4
              delay=200
            }}
          ><Icon
              @name={{outcomeIcon this.outcome.key}}
              @size={{12}}
            />{{this.outcome.label}}</span>
        {{/if}}
        {{#if @vote.result_text}}
          <span class="dis-vote-result">{{@vote.result_text}}</span>
        {{/if}}
        {{#if (hasTally @vote)}}
          <VoteBar
            class="dis-vote-bar"
            @ja={{@vote.ja}}
            @nein={{@vote.nein}}
            @enthaltung={{@vote.enthaltung}}
          />
        {{/if}}
      </div>

      {{#if this.seats.length}}
        <div class="dis-vote-roll">
          <div class="dis-vote-chart">
            <div class="dis-vote-chart-head">
              <h5 class="dis-vote-title"><Icon
                  @name="vote"
                  @size={{14}}
                />Namentliche Abstimmung ({{num this.seats.length}})</h5>
              <CanonSegmented
                class="dis-vote-mode"
                @items={{MODES}}
                @selected={{this.mode}}
                @onChange={{this.setMode}}
              />
            </div>
            <SeatChart
              @seats={{this.seats}}
              @mode={{this.chartMode}}
              @label="Namentliche Abstimmung"
              @onPick={{this.pick}}
            />
            {{#if this.absent}}
              <p class="dis-vote-absent">{{num this.absent}}
                {{voteLabel "abwesend"}}</p>
            {{/if}}
          </div>

          {{#if this.byParty.length}}
            <div class="dis-vote-parties">
              <h5 class="dis-vote-title"><Icon
                  @name="users"
                  @size={{14}}
                />Fraktionen
                {{#if this.dissenters}}
                  <span
                    class="dis-vote-dissent-n"
                    {{rough
                      "circle"
                      color="--dis-highlight"
                      padding=5
                      delay=500
                    }}
                  ><Icon @name="git-fork" @size={{12}} />{{num this.dissenters}}
                    abweichend</span>
                {{/if}}
              </h5>
              <ul class="dis-vote-party-list">
                {{#each this.byParty key="code" as |p|}}
                  <li
                    class="dis-vote-party
                      {{if p.dissent.length 'dis-vote-party--split'}}"
                  >
                    <span class="dis-vote-party-code">
                      {{#if p.code}}
                        <Tag @mark={{p.mark}} title={{p.name}}>{{p.code}}</Tag>
                      {{else}}
                        <Tag>—</Tag>
                      {{/if}}
                    </span>
                    <VoteBar
                      class="dis-vote-party-bar"
                      @ja={{p.n.ja}}
                      @nein={{p.n.nein}}
                      @enthaltung={{p.n.enthaltung}}
                    />
                    <span
                      class="dis-vote-party-who"
                      {{rough
                        "bracket"
                        color="--dis-highlight"
                        show=p.split
                        brackets="left"
                        padding=4
                        strokeWidth=2
                        delay=800
                      }}
                    >
                      {{#each p.dissent as |m|}}
                        {{#if m.person_id}}
                          <button
                            type="button"
                            class="dis-vote-dissenter dis-vote-dissenter--{{m.vote}}"
                            title={{voteLabel m.vote}}
                            {{on "click" (fn this.pick m.person_id)}}
                          >{{m.name}}
                            <span class="dis-vote-dissenter-v">{{voteLabel
                                m.vote
                              }}</span></button>
                        {{else}}
                          <span
                            class="dis-vote-dissenter dis-vote-dissenter--{{m.vote}}"
                          >{{m.name}}
                            <span class="dis-vote-dissenter-v">{{voteLabel
                                m.vote
                              }}</span></span>
                        {{/if}}
                      {{else}}
                        {{#if (several p)}}
                          <span class="dis-vote-closed">Geschlossen</span>
                        {{/if}}
                      {{/each}}
                    </span>
                  </li>
                {{/each}}
              </ul>
            </div>
          {{/if}}
        </div>
      {{/if}}

      {{#if (isHttp @vote.image_url)}}
        <a
          class="dis-vote-image"
          href={{@vote.image_url}}
          target="_blank"
          rel="noopener noreferrer"
        >
          Abstimmungsbild
          <Icon @name="launch" @size={{12}} />
        </a>
      {{/if}}
    </div>
  </template>
}
