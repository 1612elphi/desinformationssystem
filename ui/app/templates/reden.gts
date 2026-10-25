import Component from '@glimmer/component';
import { cached, tracked } from '@glimmer/tracking';
import { concat, fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { service } from '@ember/service';
import { htmlSafe } from '@ember/template';
import { pageTitle } from 'ember-page-title';
import CanonBar from 'delphicanon/components/canon-bar';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonPage from 'delphicanon/components/canon-page';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonTiles from 'delphicanon/components/canon-tiles';
import type RedenController from 'ui/controllers/reden';
import Avatar from 'ui/components/avatar';
import DataTable, { type Column } from 'ui/components/data-table';
import { SelectField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import SeatChart, { type Seat } from 'ui/components/seat-chart';
import Tag from 'ui/components/tag';
import VoteBar from 'ui/components/vote-bar';
import rough from 'ui/modifiers/rough';
import shortcut from 'ui/modifiers/shortcut';
import type NavService from 'ui/services/nav';
import {
  getPersonIds,
  mins,
  type SaidVotedRow,
  type SpeechAnalytics,
  type SpeechParty,
  type SpeechTop,
} from 'ui/utils/api-reden';
import { partyMark } from 'ui/utils/api-menschen';
import { dateDe } from 'ui/utils/calendar';
import Load from 'ui/utils/load';
import { eq, num, pct } from 'ui/utils/meta';
import './reden.css';

const TONES = ['kooperativ', 'sachlich', 'kritisch', 'polemisch'];
const STANCE: Record<string, string> = {
  dafuer: 'dafür',
  dagegen: 'dagegen',
  gemischt: 'gemischt',
  keine: 'keine',
};
const VOTE: Record<string, string> = {
  ja: 'Ja',
  nein: 'Nein',
  enthaltung: 'Enth.',
  abwesend: 'abw.',
};

const METHOD_NOTE =
  'Grundlage sind die transkribierten Livestreams des Gemeinderats. Haltung, Tonlage, Stimmung und Bezüge sind Schätzungen des Sprachmodells Jev, keine geprüften Fakten.';

const SAID_VOTED_NOTE =
  'Die Haltung in der Rede (laut Jev) neben der eigenen Stimme zum selben Punkt. Gemeinsam beratene Punkte und unsichere Lesungen zählen nicht mit.';

const SV_COLUMNS: Column[] = [
  { key: 'speaker', label: 'Redner:in' },
  { key: 'party', label: 'Fraktion', width: '6rem' },
  { key: 'top', label: 'TOP', width: '5rem' },
  { key: 'stance', label: 'Rede', width: '6rem' },
  { key: 'vote', label: 'Stimme', width: '6rem' },
  { key: 'match', label: 'Abgleich', width: '8rem' },
];

const SV_PREVIEW = 12;

const TOP_COLUMNS: Column[] = [
  { key: 'number', label: 'TOP', width: '4rem' },
  { key: 'title', label: 'Thema' },
  { key: 'secs', label: 'Dauer', width: '9rem' },
  { key: 'speeches', label: 'Reden', width: '4.5rem', cls: 'dis-num' },
  { key: 'parties', label: 'Fraktionen', width: '6rem', cls: 'dis-num' },
  { key: 'vote', label: 'Abstimmung', width: '10rem' },
];

const signed = (x?: number | null) =>
  x == null
    ? '—'
    : `${x > 0 ? '+' : x < 0 ? '−' : '±'}${Math.abs(x).toFixed(2)}`;
const css = (s: string) => htmlSafe(s);
const fill = (share: number, code: string) =>
  css(`inline-size: ${share * 100}%; background: ${partyMark(code)}`);
const at = (share: number) => css(`inset-inline-start: ${share * 100}%`);
const grow = (n?: number) => css(`flex-grow: ${n ?? 0}`);

// gold for agreeing, red for rejecting, as on the voting panel; |v| below 0.02 is neutral
function diverging(v: number | null | undefined, max = 1) {
  if (v == null || Math.abs(v) < 0.02) return undefined;
  const c = v > 0 ? 'var(--dis-yes)' : 'var(--dis-no)';
  const p = Math.round(15 + 70 * Math.min(1, Math.abs(v) / max));
  return css(`background: color-mix(in srgb, ${c} ${p}%, var(--canon-card))`);
}

// sentiment -1..1 as a bar growing left or right from the centre
function mood(v?: number | null) {
  const x = Math.max(-1, Math.min(1, v ?? 0));
  const c = x >= 0 ? 'var(--dis-yes)' : 'var(--dis-no)';
  return css(
    `inset-inline-start: ${50 + Math.min(0, x) * 50}%; inline-size: ${Math.abs(x) * 50}%; background: ${c}`,
  );
}

interface Signature {
  Args: { controller: RedenController };
}

export default class RedenTemplate extends Component<Signature> {
  @service declare nav: NavService;

  // a party picked in the floor-time chart narrows speakers, said-vs-voted and words
  @tracked party = '';

  ids = new Load(getPersonIds());

  get c() {
    return this.args.controller;
  }

  get data(): SpeechAnalytics | undefined {
    return this.c.load.value;
  }

  get meetingItems() {
    return [
      { id: '', label: 'Alle Sitzungen' },
      ...this.c.meetings.map((m) => ({
        id: m.id,
        label: `${dateDe(m.date)} · ${m.body_name}`,
      })),
    ];
  }

  get partial() {
    const t = this.data?.totals;
    return !!t && t.judged < t.speeches;
  }

  get tiles() {
    const t = this.data?.totals;
    if (!t) return [];
    return [
      { label: 'Reden', value: num(t.speeches) },
      {
        label: 'Stunden',
        value: (t.secs / 3600).toLocaleString('de-DE', {
          maximumFractionDigits: 1,
        }),
      },
      { label: 'Wörter', value: num(t.words) },
      { label: 'Redner:innen', value: num(t.speakers) },
      { label: 'Sitzungen', value: num(t.meetings) },
    ];
  }

  // meetings are listed newest first
  step = (by: number) => () => {
    const ids = ['', ...this.c.meetings.map((m) => m.id)];
    const i = ids.indexOf(this.c.meeting);
    const next = ids[Math.min(ids.length - 1, Math.max(0, i + by))];
    if (next !== undefined) this.c.setMeeting(next);
  };

  pickParty = (code: string) => (this.party = this.party === code ? '' : code);
  clearParty = () => (this.party = '');
  dim = (code: string) => !!this.party && code !== this.party;
  hasParty = (code: string) => !this.party || code === this.party;

  personId = (name?: string | null) =>
    name ? this.ids.value?.get(name) : undefined;
  openPerson = (name: string) => {
    const id = this.personId(name);
    if (id) this.nav.open({ person: id });
  };
  pickSeat = (id: string) => this.nav.open({ person: id });
  openMeeting = (id: string) => this.nav.open({ meeting: id });
  openSelected = () => this.openMeeting(this.c.meeting);
  openTop = (t: SpeechTop) => this.openMeeting(t.meeting_id);
  topLabel = (t: SpeechTop) => `TOP ${t.number}: ${t.title}`;

  @cached
  get floor() {
    const rows = (this.data?.parties ?? []).filter((p) => p.code !== 'OB');
    const max = Math.max(
      0.01,
      ...rows.map((p) => Math.max(p.time_share ?? 0, p.seat_share ?? 0)),
    );
    return rows.map((p) => ({
      p,
      time: (p.time_share ?? 0) / max,
      seat: (p.seat_share ?? 0) / max,
      over: (p.time_share ?? 0) > (p.seat_share ?? 0),
    }));
  }

  // the party furthest over its seat share
  get loudest() {
    let best = '';
    let most = 0;
    for (const f of this.floor) {
      const d = (f.p.time_share ?? 0) - (f.p.seat_share ?? 0);
      if (d > most) [best, most] = [f.p.code, d];
    }
    return best;
  }

  @cached
  get seats(): Seat[] {
    return (this.data?.speakers ?? [])
      .filter((s) => this.hasParty(s.party))
      .map((s) => ({
        name: s.name,
        party_code: s.party,
        person_id: this.personId(s.name) ?? null,
      }));
  }

  @cached
  get speakers() {
    const top = (this.data?.speakers ?? [])
      .filter((s) => this.hasParty(s.party))
      .slice(0, 15);
    const max = Math.max(1, ...top.map((s) => s.secs));
    return top.map((s) => ({ s, share: s.secs / max }));
  }

  get sv() {
    return this.data?.said_voted;
  }

  get svTiles() {
    const sv = this.sv;
    if (!sv) return [];
    return [
      {
        label: 'Konsistent',
        value: sv.n ? pct(sv.match / sv.n) : '—',
        level: sv.n ? sv.match / sv.n : undefined,
      },
      { label: 'Abweichungen', value: num(sv.n - sv.match) },
      { label: 'Vergleichbar', value: num(sv.n) },
    ];
  }

  @tracked svAll = false;

  // contradictions first, then agreements, then the rows that do not count
  @cached
  get svRows() {
    const rank = (r: SaidVotedRow) =>
      r.match === false ? 0 : r.match === true ? 1 : 2;
    return (this.sv?.rows ?? [])
      .filter((r) => r.vote && this.hasParty(r.party))
      .sort((a, b) => rank(a) - rank(b));
  }

  get svShown() {
    return this.svAll ? this.svRows : this.svRows.slice(0, SV_PREVIEW);
  }

  get svMore() {
    return this.svRows.length > SV_PREVIEW;
  }

  toggleSv = () => (this.svAll = !this.svAll);

  topOf = (r: SaidVotedRow) => r.anchor.replace('top', '');
  stance = (r: SaidVotedRow) => STANCE[r.stance] ?? r.stance;
  vote = (r: SaidVotedRow) => VOTE[r.vote ?? ''] ?? r.vote;
  jev = (r: SaidVotedRow) => `Jev ${Math.round(r.p * 100)} %`;

  get toneRows() {
    return (this.data?.parties ?? []).filter((p) => p.tone);
  }

  tone = (p: SpeechParty, t: string) => p.tone?.[t] ?? 0;
  toneTitle = (p: SpeechParty, t: string) =>
    `${p.code}, ${t}: ${pct(this.tone(p, t))}`;

  get codes() {
    return (this.data?.parties ?? [])
      .map((p) => p.code)
      .filter((c) => c !== 'OB');
  }

  @cached
  get mentions() {
    return new Map(
      (this.data?.matrix ?? []).map((e) => [`${e.from}|${e.to}`, e]),
    );
  }

  cell = (row: string, col: string) => {
    const e = this.mentions.get(`${row}|${col}`);
    if (!e) return { text: '', title: '', style: undefined };
    return {
      text: num(e.n),
      title: `${row} über ${col}: ${e.n}× · Haltung ${signed(e.attitude)}`,
      style: diverging(e.attitude, 0.6),
    };
  };

  @cached
  get cards() {
    const dist = this.data?.words.distinctive ?? {};
    return (this.data?.parties ?? [])
      .map((p) => p.code)
      .filter((c) => dist[c]?.length && this.hasParty(c))
      .map((code) => {
        const ws = dist[code]!.slice(0, 8);
        const maxZ = Math.max(0.1, ...ws.map((w) => w.z));
        return {
          code,
          words: ws.map((w) => ({
            ...w,
            share: w.z / maxZ,
            title: `${w.word}: ${w.n}× · z ${w.z}`,
          })),
        };
      });
  }

  @cached
  get cloud() {
    const ws = (this.data?.words.overall ?? []).slice(0, 24);
    const max = Math.max(1, ...ws.map((w) => w.n));
    return ws.map((w) => ({
      ...w,
      style: css(`font-size: ${(0.8 + 1.5 * (w.n / max)).toFixed(2)}rem`),
    }));
  }

  get readable() {
    return (this.data?.parties ?? []).filter(
      (p) => p.flesch != null && p.words > 0,
    );
  }

  flesch = (p: SpeechParty) => Math.max(0, Math.min(100, p.flesch ?? 0)) / 100;

  get maxTop() {
    return Math.max(1, ...(this.data?.tops ?? []).map((t) => t.secs));
  }

  topShare = (t: SpeechTop) => t.secs / this.maxTop;

  <template>
    {{pageTitle "Reden"}}
    <CanonPage class="dis-reden" @maxWidth={{1080}}>
      <h1 class="dis-visually-hidden">Reden</h1>
      <div
        class="dis-reden-filter"
        {{shortcut "[" (this.step 1)}}
        {{shortcut "]" (this.step -1)}}
      >
        <SelectField
          @label="Sitzung"
          @items={{this.meetingItems}}
          @value={{this.c.meeting}}
          @onChange={{this.c.setMeeting}}
        />
        <div class="dis-reden-filter-actions">
          {{#if this.c.meeting}}
            <CanonButton
              @kind="cell"
              @text="Sitzung öffnen"
              @icon={{component Icon name="calendar"}}
              @onClick={{this.openSelected}}
            />
          {{/if}}
          {{#if this.party}}
            <button
              type="button"
              class="dis-reden-chip"
              aria-label="Alle Fraktionen"
              {{on "click" this.clearParty}}
              {{rough "circle" color="--dis-highlight" padding=6}}
            >
              <PartyChip @code={{this.party}} @party={{this.party}} />
              <Icon @name="close" @size={{14}} />
            </button>
          {{/if}}
          {{#if this.partial}}
            <Tag><Icon @name="bot" @size={{12}} />
              Jev
              {{num this.data.totals.judged}}/{{num
                this.data.totals.speeches
              }}</Tag>
          {{/if}}
          <p class="dis-reden-keys" aria-hidden="true">
            <kbd>[</kbd><kbd>]</kbd>
            Sitzung
          </p>
        </div>
      </div>
      <p class="dis-reden-note dis-reden-method"><Icon
          @name="sparkles"
          @size={{14}}
        />{{METHOD_NOTE}}</p>

      <LoadState @load={{this.c.load}}>
        {{#if this.data.totals.speeches}}
          <CanonRows @framed={{true}}>
            <CanonTiles @items={{this.tiles}} />
          </CanonRows>

          <section class="dis-reden-section">
            <h2 class="dis-reden-heading"><Icon @name="timer" />Redezeit und
              Sitze</h2>
            <div class="dis-reden-floor">
              <div>
                <p class="dis-reden-legend">
                  <span class="dis-reden-key dis-reden-key--bar"></span>
                  Redeanteil
                  <span class="dis-reden-key dis-reden-key--tick"></span>
                  Sitzanteil
                </p>
                <CanonRows @framed={{true}}>
                  {{#each this.floor key="p.code" as |f|}}
                    <button
                      type="button"
                      class="dis-reden-row {{if (this.dim f.p.code) 'dis-dim'}}"
                      title="{{f.p.code}}: {{mins f.p.secs}}, {{num
                        f.p.speeches
                      }} Reden, {{num f.p.seats}} Sitze"
                      aria-pressed={{if
                        (eq this.party f.p.code)
                        "true"
                        "false"
                      }}
                      {{on "click" (fn this.pickParty f.p.code)}}
                    >
                      <span class="dis-reden-label"><PartyChip
                          @code={{f.p.code}}
                          @party={{f.p.code}}
                        /></span>
                      <span class="dis-reden-track" aria-hidden="true">
                        <span
                          class="dis-reden-fill"
                          style={{fill f.time f.p.code}}
                        ></span>
                        <span class="dis-reden-tick" style={{at f.seat}}></span>
                      </span>
                      <span
                        class="dis-reden-value {{if f.over 'dis-reden-over'}}"
                      ><span
                          {{rough
                            "underline"
                            strokeWidth=2
                            padding=1
                            show=(eq f.p.code this.loudest)
                            delay=300
                          }}
                        >{{pct f.p.time_share}}</span></span>
                      <span class="dis-reden-meta">{{pct f.p.seat_share}}
                        Sitze ·
                        {{mins f.p.secs}}</span>
                    </button>
                  {{/each}}
                </CanonRows>
              </div>
              <figure class="dis-reden-hall">
                <SeatChart
                  @seats={{this.seats}}
                  @mode="party"
                  @label="Redner:innen im Saal"
                  @onPick={{this.pickSeat}}
                />
                <figcaption class="dis-reden-legend">{{num this.seats.length}}
                  Redner:innen</figcaption>
              </figure>
            </div>
          </section>

          <section class="dis-reden-section">
            <h2 class="dis-reden-heading"><Icon @name="megaphone" />Wer am
              meisten redet</h2>
            <CanonRows @framed={{true}}>
              {{#each this.speakers key="s.name" as |x i|}}
                {{#let (this.personId x.s.name) as |id|}}
                  <button
                    type="button"
                    class="dis-reden-row dis-reden-speaker"
                    title="{{x.s.name}}: {{num x.s.speeches}} Reden, {{num
                      x.s.words
                    }} Wörter"
                    disabled={{unless id true}}
                    {{on "click" (fn this.openPerson x.s.name)}}
                  >
                    <Avatar
                      class="dis-reden-avatar"
                      @personId={{id}}
                      @name={{x.s.name}}
                    />
                    <span class="dis-reden-name">{{x.s.name}}</span>
                    <span class="dis-reden-label"><PartyChip
                        @code={{x.s.party}}
                        @party={{x.s.party}}
                      /></span>
                    <CanonBar
                      class="dis-reden-bar"
                      aria-hidden="true"
                      @share={{x.share}}
                      @fill={{partyMark x.s.party}}
                    />
                    <span class="dis-reden-value"><span
                        {{rough "circle" show=(eq i 0) padding=5 delay=600}}
                      >{{mins x.s.secs}}</span></span>
                  </button>
                {{/let}}
              {{else}}
                <CanonEmptyRow @text="Keine Reden" />
              {{/each}}
            </CanonRows>
          </section>

          {{#if this.sv.n}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon @name="scale" />Gesagt vs.
                gestimmt</h2>
              <p class="dis-reden-note">{{SAID_VOTED_NOTE}}</p>
              <CanonRows @framed={{true}}>
                <CanonTiles @items={{this.svTiles}} />
              </CanonRows>
              <DataTable
                class="dis-reden-sv"
                @columns={{SV_COLUMNS}}
                @rows={{this.svShown}}
                as |r col|
              >
                {{#if (eq col.key "speaker")}}
                  <button
                    type="button"
                    class="dis-reden-link"
                    {{on "click" (fn this.openPerson r.speaker)}}
                  >{{r.speaker}}</button>
                {{else if (eq col.key "party")}}
                  <PartyChip @code={{r.party}} @party={{r.party}} />
                {{else if (eq col.key "top")}}
                  <button
                    type="button"
                    class="dis-reden-link"
                    {{on "click" (fn this.openMeeting r.meeting_id)}}
                  >{{this.topOf r}}</button>
                {{else if (eq col.key "stance")}}
                  <span title={{this.jev r}}>{{this.stance r}}</span>
                {{else if (eq col.key "vote")}}
                  <span
                    class="dis-reden-vote dis-reden-vote--{{r.vote}}"
                  >{{this.vote r}}</span>
                {{else}}
                  <span class="dis-reden-match">
                    {{#if (eq r.match true)}}
                      <span class="dis-reden-hit"><Icon
                          @name="checkmark"
                        /><span class="dis-visually-hidden">passt</span></span>
                    {{else if (eq r.match false)}}
                      <span class="dis-reden-miss"><Icon @name="close" /><span
                          class="dis-visually-hidden"
                        >weicht ab</span></span>
                    {{/if}}
                    {{#if r.joint}}<Tag>gemeinsam</Tag>{{/if}}
                  </span>
                {{/if}}
              </DataTable>
              {{#if this.svMore}}
                <CanonButton
                  class="dis-reden-more"
                  @kind="cell"
                  @text={{if
                    this.svAll
                    "Weniger"
                    (concat "Alle zeigen (" (num this.svRows.length) ")")
                  }}
                  @onClick={{this.toggleSv}}
                />
              {{/if}}
            </section>
          {{/if}}

          {{#if this.toneRows.length}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon @name="thermometer" />Tonlage
                und Stimmung</h2>
              <p class="dis-reden-legend">
                {{#each TONES as |t|}}
                  <span class="dis-reden-key dis-reden-tone--{{t}}"></span>{{t}}
                {{/each}}
              </p>
              <CanonRows @framed={{true}}>
                {{#each this.toneRows key="code" as |p|}}
                  <div
                    class="dis-reden-row dis-reden-tone
                      {{if (this.dim p.code) 'dis-dim'}}"
                  >
                    <span class="dis-reden-label"><PartyChip
                        @code={{p.code}}
                        @party={{p.code}}
                      /></span>
                    <span class="dis-reden-stack">
                      {{#each TONES as |t|}}
                        <span
                          class="dis-reden-seg dis-reden-tone--{{t}}"
                          style={{grow (this.tone p t)}}
                          title={{this.toneTitle p t}}
                        ></span>
                      {{/each}}
                    </span>
                    <span
                      class="dis-reden-mood"
                      role="img"
                      aria-label="Stimmung {{signed p.sentiment}}"
                    >
                      <span
                        class="dis-reden-mood-fill"
                        style={{mood p.sentiment}}
                      ></span>
                    </span>
                    <span class="dis-reden-value">{{signed p.sentiment}}</span>
                  </div>
                {{/each}}
              </CanonRows>
            </section>
          {{/if}}

          {{#if this.data.matrix.length}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon @name="messages-square" />Wer
                über wen</h2>
              <div class="dis-reden-matrix-wrap">
                <table class="dis-reden-matrix">
                  <thead>
                    <tr>
                      <th scope="col">von ↓ über →</th>
                      {{#each this.codes as |code|}}
                        <th scope="col">{{code}}</th>
                      {{/each}}
                    </tr>
                  </thead>
                  <tbody>
                    {{#each this.codes as |row|}}
                      <tr class={{if (this.dim row) "dis-dim"}}>
                        <th scope="row">{{row}}</th>
                        {{#each this.codes as |col|}}
                          {{#if (eq row col)}}
                            <td class="dis-reden-self"></td>
                          {{else}}
                            {{#let (this.cell row col) as |x|}}
                              <td
                                title={{x.title}}
                                style={{x.style}}
                              >{{x.text}}</td>
                            {{/let}}
                          {{/if}}
                        {{/each}}
                      </tr>
                    {{/each}}
                  </tbody>
                </table>
              </div>
              <p class="dis-reden-legend">
                <span class="dis-reden-key dis-reden-key--neg"></span>
                ablehnend
                <span class="dis-reden-key dis-reden-key--none"></span>
                neutral
                <span class="dis-reden-key dis-reden-key--pos"></span>
                zustimmend
              </p>
            </section>
          {{/if}}

          {{#if this.cards.length}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon
                  @name="message-square-quote"
                />Typische Wörter</h2>
              <div class="dis-reden-cards">
                {{#each this.cards key="code" as |card|}}
                  <div class="dis-reden-card">
                    <PartyChip @code={{card.code}} @party={{card.code}} />
                    {{#each card.words key="word" as |w|}}
                      <div class="dis-reden-word" title={{w.title}}>
                        <span>{{w.word}}</span>
                        <CanonBar
                          class="dis-reden-wbar"
                          aria-hidden="true"
                          @share={{w.share}}
                          @fill={{partyMark card.code}}
                          @track="transparent"
                        />
                      </div>
                    {{/each}}
                  </div>
                {{/each}}
              </div>
            </section>
          {{/if}}

          {{#if this.cloud.length}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon @name="cloud" />Häufigste
                Wörter</h2>
              <ul class="dis-reden-cloud">
                {{#each this.cloud key="word" as |w i|}}
                  <li
                    style={{w.style}}
                    title="{{w.word}}: {{w.n}}×"
                    {{rough
                      "circle"
                      color="--dis-highlight"
                      show=(eq i 0)
                      padding=8
                    }}
                  >
                    {{w.word}}<span class="dis-reden-cloud-n">{{num w.n}}</span>
                  </li>
                {{/each}}
              </ul>
            </section>
          {{/if}}

          {{#if this.readable.length}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon
                  @name="book-open"
                />Lesbarkeit</h2>
              <CanonRows @framed={{true}}>
                {{#each this.readable key="code" as |p|}}
                  <div
                    class="dis-reden-row {{if (this.dim p.code) 'dis-dim'}}"
                    title="{{p.code}}: Flesch {{num p.flesch}}, {{num
                      p.sentence_len
                    }} Wörter/Satz"
                  >
                    <span class="dis-reden-label"><PartyChip
                        @code={{p.code}}
                        @party={{p.code}}
                      /></span>
                    <CanonBar
                      class="dis-reden-bar"
                      aria-hidden="true"
                      @share={{this.flesch p}}
                      @fill={{partyMark p.code}}
                    />
                    <span class="dis-reden-value">{{num p.flesch}}</span>
                    <span class="dis-reden-meta">{{num p.sentence_len}}
                      Wörter/Satz</span>
                  </div>
                {{/each}}
              </CanonRows>
            </section>
          {{/if}}

          {{#if this.data.tops.length}}
            <section class="dis-reden-section">
              <h2 class="dis-reden-heading"><Icon @name="gavel" />Debatten je
                TOP</h2>
              <DataTable
                class="dis-reden-tops"
                @columns={{TOP_COLUMNS}}
                @rows={{this.data.tops}}
                @onOpen={{this.openTop}}
                @rowLabel={{this.topLabel}}
                as |t col|
              >
                {{#if (eq col.key "number")}}
                  {{t.number}}
                {{else if (eq col.key "title")}}
                  <span class="dis-reden-top-title">{{t.title}}
                    {{#unless this.c.meeting}}<span
                        class="dis-reden-meta"
                      >{{dateDe t.date}}</span>{{/unless}}</span>
                {{else if (eq col.key "secs")}}
                  <span class="dis-reden-dur">
                    <CanonBar
                      class="dis-reden-bar"
                      aria-hidden="true"
                      @share={{this.topShare t}}
                    />
                    {{mins t.secs}}
                  </span>
                {{else if (eq col.key "speeches")}}
                  {{num t.speeches}}
                {{else if (eq col.key "parties")}}
                  {{num t.parties}}
                {{else if (eq t.ja null)}}
                  —
                {{else}}
                  <span class="dis-reden-dur">
                    <VoteBar
                      class="dis-reden-votebar"
                      @compact={{true}}
                      @ja={{t.ja}}
                      @nein={{t.nein}}
                      @enthaltung={{t.enthaltung}}
                    />
                    {{t.ja}}·{{t.nein}}·{{t.enthaltung}}
                  </span>
                {{/if}}
              </DataTable>
            </section>
          {{/if}}
        {{else}}
          <CanonRows @framed={{true}}>
            <CanonEmptyRow @text="Keine Reden" />
          </CanonRows>
        {{/if}}
      </LoadState>
    </CanonPage>
  </template>
}
