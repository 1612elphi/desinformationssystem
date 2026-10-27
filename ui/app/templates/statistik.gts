import Component from '@glimmer/component';
import { cached, tracked } from '@glimmer/tracking';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { service } from '@ember/service';
import { htmlSafe } from '@ember/template';
import { pageTitle } from 'ember-page-title';
import CanonBar from 'delphicanon/components/canon-bar';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonPage from 'delphicanon/components/canon-page';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonTiles from 'delphicanon/components/canon-tiles';
import type { Analytics, AnalyticsParty } from 'ui/api';
import { LoadState } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import Icon from 'ui/components/icon';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import { getPeople } from 'ui/utils/api-shell';
import Load from 'ui/utils/load';
import { eq, num, pct, SEAT_ORDER } from 'ui/utils/meta';
import './statistik.css';

type Dissenter = Analytics['dissenters'][number] & { rate: number };
type SortKey = 'name' | 'party' | 'dissents' | 'votes' | 'rate';

const DISSENTER_COLUMNS: { key: SortKey; label: string; num?: boolean }[] = [
  { key: 'name', label: 'Name (Panel)' },
  { key: 'party', label: 'Fraktion' },
  { key: 'dissents', label: 'Abweichungen', num: true },
  { key: 'votes', label: 'Stimmen', num: true },
  { key: 'rate', label: 'Quote', num: true },
];

const basis = (n: number | null | undefined) =>
  htmlSafe(`flex-basis: ${(n ?? 0) * 100}%`);
const seat = (code: string) => {
  const i = SEAT_ORDER.indexOf(code);
  return i < 0 ? SEAT_ORDER.length : i;
};

interface Signature {
  Args: { model: Load<Analytics> };
}

export default class StatistikTemplate extends Component<Signature> {
  @service declare nav: NavService;

  @tracked hover = '';
  @tracked selected = '';
  @tracked cellRow = '';
  @tracked cellCol = '';
  @tracked sortKey: SortKey = 'dissents';
  @tracked sortAsc = false;

  people = new Load(getPeople());

  get data() {
    return this.args.model.value;
  }

  get focus() {
    return this.hover || this.selected;
  }

  get tiles() {
    const t = this.data?.totals;
    if (!t) return [];
    return [
      { label: 'Abstimmungen', value: num(t.votes) },
      { label: 'Namentlich', value: num(t.with_rollcall) },
      { label: 'Einstimmig', value: this.share(t.unanimous, t.votes) },
      { label: 'Mit Gegenstimmen', value: this.share(t.contested, t.votes) },
    ];
  }

  share = (part: number, whole: number) => (whole ? pct(part / whole) : '—');

  @cached
  get parties() {
    return [...(this.data?.parties ?? [])].sort(
      (a, b) => seat(a.code) - seat(b.code),
    );
  }

  get maxCast() {
    return Math.max(
      1,
      ...this.parties.map((p) => p.ja + p.nein + p.enthaltung),
    );
  }

  segment = (n: number) => basis(n / this.maxCast);

  cohesion = (p: AnalyticsParty) => p.cohesion ?? 0;

  get loosest() {
    const rated = this.parties.filter((p) => p.cohesion != null);
    if (rated.length < 2) return '';
    return rated.reduce((a, b) => (this.cohesion(b) < this.cohesion(a) ? b : a))
      .code;
  }

  @cached
  get closest() {
    let best: { a: string; b: string; agree: number } | undefined;
    for (const x of this.data?.agreement ?? [])
      if (!best || x.agree > best.agree) best = x;
    if (!best) return '';
    const [a, b] = [best.a, best.b].sort((x, y) => seat(x) - seat(y));
    return `${a}|${b}`;
  }

  isClosest = (row: string, col: string) => this.closest === `${row}|${col}`;

  get topDissents() {
    return Math.max(0, ...(this.data?.dissenters ?? []).map((d) => d.dissents));
  }

  isTopDissenter = (d: Dissenter) =>
    d.dissents > 0 && d.dissents === this.topDissents;

  get codes() {
    return this.parties.map((p) => p.code);
  }

  get showMatrix() {
    return this.codes.length >= 2;
  }

  @cached
  get agreement() {
    const m = new Map<string, { agree: number; n: number }>();
    for (const a of this.data?.agreement ?? []) {
      m.set(`${a.a}|${a.b}`, a);
      m.set(`${a.b}|${a.a}`, a);
    }
    return m;
  }

  cell = (row: string, col: string) => {
    const c = this.agreement.get(`${row}|${col}`);
    if (!c)
      return {
        title: 'keine gemeinsamen Abstimmungen',
        text: '',
        style: undefined,
      };
    return {
      title: `${row} & ${col}: ${pct(c.agree)} (${num(c.n)} gemeinsame Abstimmungen)`,
      text: String(Math.round(c.agree * 100)),
      style: htmlSafe(
        `--dis-agree: color-mix(in srgb, var(--canon-primary) ${Math.round(5 + c.agree * 50)}%, var(--canon-card))`,
      ),
    };
  };

  rowLit = (code: string) => code === (this.cellRow || this.focus);
  colLit = (code: string) => code === (this.cellCol || this.focus);
  cellLit = (row: string, col: string) => this.rowLit(row) || this.colLit(col);
  isHot = (code: string) => code === this.focus;
  dim = (code: string) => !!this.focus && code !== this.focus;

  get allyParty() {
    return this.focus || this.codes[0] || '';
  }

  get allies() {
    const p = this.allyParty;
    return this.codes
      .filter((c) => c !== p)
      .map((c) => ({ code: c, agree: this.agreement.get(`${p}|${c}`) }))
      .filter((x) => x.agree)
      .sort((a, b) => (b.agree?.agree ?? 0) - (a.agree?.agree ?? 0))
      .map((x) => ({
        code: x.code,
        share: x.agree?.agree ?? 0,
        n: x.agree?.n ?? 0,
      }));
  }

  enter = (code: string) => (this.hover = code);
  leave = () => (this.hover = '');
  enterCell = (row: string, col: string) => {
    this.cellRow = row;
    this.cellCol = col;
  };
  leaveCell = () => {
    this.cellRow = '';
    this.cellCol = '';
  };
  toggle = (code: string) =>
    (this.selected = this.selected === code ? '' : code);
  clear = () => (this.selected = '');

  @cached
  get dissenters(): Dissenter[] {
    const rows = (this.data?.dissenters ?? [])
      .filter((d) => !this.selected || d.party === this.selected)
      .map((d) => ({ ...d, rate: d.votes ? d.dissents / d.votes : 0 }));
    const k = this.sortKey;
    const dir = this.sortAsc ? 1 : -1;
    return rows.sort((a, b) => {
      const x = a[k];
      const y = b[k];
      return (
        dir *
        (typeof x === 'string'
          ? x.localeCompare(y as string, 'de')
          : x - (y as number))
      );
    });
  }

  get maxRate() {
    return Math.max(0.01, ...this.dissenters.map((d) => d.rate));
  }

  rateShare = (d: Dissenter) => d.rate / this.maxRate;

  sortBy = (key: SortKey) => {
    if (this.sortKey === key) this.sortAsc = !this.sortAsc;
    else {
      this.sortKey = key;
      this.sortAsc = key === 'name' || key === 'party';
    }
  };

  ariaSort = (key: SortKey) =>
    this.sortKey === key ? (this.sortAsc ? 'ascending' : 'descending') : 'none';
  sortMark = (key: SortKey) =>
    this.sortKey === key ? (this.sortAsc ? '↑' : '↓') : '';

  // panel records surnames only
  personFor = (d: Dissenter) => {
    const hits = (this.people.value ?? []).filter(
      (p) => p.party_code === d.party && p.name.split(' ').at(-1) === d.name,
    );
    return hits.length === 1 ? hits[0]?.person_id : undefined;
  };

  openPerson = (id: string) => this.nav.open({ person: id });

  <template>
    {{pageTitle "Statistik"}}
    <CanonPage class="dis-statistik" @maxWidth={{1080}}>
      <h1 class="dis-visually-hidden">Statistik</h1>
      <LoadState @load={{@model}}>
        {{#if this.data}}
          {{#let this.data as |data|}}
            <CanonRows @framed={{true}}>
              <CanonTiles @items={{this.tiles}} />
            </CanonRows>

            {{#if data.parties.length}}
              <section class="dis-stat-section">
                <h2 class="dis-stat-heading"><Icon
                    @name="magnet"
                    @size={{20}}
                  />Fraktionsdisziplin</h2>
                <p class="dis-stat-note">
                  Anteil der abgegebenen Stimmen, die der Mehrheitslinie der
                  eigenen Fraktion folgen.
                </p>
                <CanonRows @framed={{true}}>
                  {{#each this.parties key="code" as |p|}}
                    <button
                      type="button"
                      class="dis-bar-row dis-party-row
                        {{if (this.isHot p.code) 'dis-hot'}}
                        {{if (this.dim p.code) 'dis-dim'}}"
                      aria-pressed={{if
                        (eq this.selected p.code)
                        "true"
                        "false"
                      }}
                      {{on "click" (fn this.toggle p.code)}}
                      {{on "mouseenter" (fn this.enter p.code)}}
                      {{on "mouseleave" this.leave}}
                      {{on "focus" (fn this.enter p.code)}}
                      {{on "blur" this.leave}}
                    >
                      <span class="dis-bar-label"><PartyChip
                          @code={{p.code}}
                          @party={{p.code}}
                          @afdChance={{1}}
                        /></span>
                      <CanonBar
                        class="dis-bar"
                        aria-hidden="true"
                        @share={{this.cohesion p}}
                      />
                      <span class="dis-bar-value"><span
                          {{rough
                            "circle"
                            show=(eq p.code this.loosest)
                            padding=6
                            delay=400
                          }}
                        >{{pct p.cohesion}}</span></span>
                      <span class="dis-bar-meta">{{num p.votes}}
                        Abst. ·
                        {{num p.members_cast}}
                        Stimmen</span>
                    </button>
                  {{/each}}
                </CanonRows>
              </section>

              <section class="dis-stat-section">
                <h2 class="dis-stat-heading"><Icon
                    @name="vote"
                    @size={{20}}
                  />Stimmverteilung</h2>
                <p class="dis-stat-note">
                  Alle zugeordneten Einzelstimmen je Fraktion (Gelb = Ja, Rot =
                  Nein, Grau = Enthaltung — wie auf dem Abstimmungspanel).
                </p>
                <CanonRows @framed={{true}}>
                  {{#each this.parties key="code" as |p|}}
                    {{! template-lint-disable no-invalid-interactive }}
                    <div
                      class="dis-bar-row
                        {{if (this.isHot p.code) 'dis-hot'}}
                        {{if (this.dim p.code) 'dis-dim'}}"
                      {{on "mouseenter" (fn this.enter p.code)}}
                      {{on "mouseleave" this.leave}}
                      {{on "click" (fn this.toggle p.code)}}
                    >
                      <span class="dis-bar-label"><PartyChip
                          @code={{p.code}}
                          @party={{p.code}}
                        /></span>
                      <span class="dis-bar dis-stack" aria-hidden="true">
                        <span
                          class="dis-seg dis-seg--ja"
                          style={{this.segment p.ja}}
                        ></span>
                        <span
                          class="dis-seg dis-seg--nein"
                          style={{this.segment p.nein}}
                        ></span>
                        <span
                          class="dis-seg dis-seg--enth"
                          style={{this.segment p.enthaltung}}
                        ></span>
                      </span>
                      <span class="dis-bar-value dis-votetext">
                        <span class="dis-vt dis-vt--ja">Ja {{num p.ja}}</span>
                        <span class="dis-vt dis-vt--nein">Nein
                          {{num p.nein}}</span>
                        <span class="dis-vt dis-vt--enth">Enth.
                          {{num p.enthaltung}}</span>
                      </span>
                    </div>
                  {{/each}}
                </CanonRows>
              </section>

              {{#if this.showMatrix}}
                <section class="dis-stat-section">
                  <h2 class="dis-stat-heading"><Icon
                      @name="handshake"
                      @size={{20}}
                    />Übereinstimmung zwischen Fraktionen</h2>
                  <p class="dis-stat-note">
                    Wie oft zwei Fraktionen bei derselben Abstimmung dieselbe
                    Mehrheitslinie hatten.
                  </p>
                  <div class="dis-agree">
                    <div class="dis-matrix-wrap">
                      <table
                        class="dis-matrix"
                        {{on "mouseleave" this.leaveCell}}
                      >
                        <thead>
                          <tr>
                            <td></td>
                            {{#each this.codes as |code|}}
                              <th
                                scope="col"
                                class={{if (this.colLit code) "dis-lit"}}
                              >
                                <button
                                  type="button"
                                  class="dis-matrix-party"
                                  aria-pressed={{if
                                    (eq this.selected code)
                                    "true"
                                    "false"
                                  }}
                                  {{on "click" (fn this.toggle code)}}
                                  {{on "mouseenter" (fn this.enter code)}}
                                  {{on "mouseleave" this.leave}}
                                >{{code}}</button>
                              </th>
                            {{/each}}
                          </tr>
                        </thead>
                        <tbody>
                          {{#each this.codes as |row|}}
                            <tr>
                              <th
                                scope="row"
                                class={{if (this.rowLit row) "dis-lit"}}
                              >
                                <button
                                  type="button"
                                  class="dis-matrix-party"
                                  tabindex="-1"
                                  {{on "click" (fn this.toggle row)}}
                                  {{on "mouseenter" (fn this.enter row)}}
                                  {{on "mouseleave" this.leave}}
                                >{{row}}</button>
                              </th>
                              {{#each this.codes as |col|}}
                                {{#if (eq row col)}}
                                  <td
                                    class="dis-matrix-self
                                      {{if (this.cellLit row col) 'dis-lit'}}"
                                  ></td>
                                {{else}}
                                  {{#let (this.cell row col) as |x|}}
                                    <td
                                      class={{if
                                        (this.cellLit row col)
                                        "dis-lit"
                                      }}
                                      title={{x.title}}
                                      style={{x.style}}
                                      {{on
                                        "mouseenter"
                                        (fn this.enterCell row col)
                                      }}
                                    ><span
                                        {{rough
                                          "circle"
                                          show=(this.isClosest row col)
                                          color="--canon-foreground"
                                          padding=6
                                          delay=1000
                                        }}
                                      >{{x.text}}</span></td>
                                  {{/let}}
                                {{/if}}
                              {{/each}}
                            </tr>
                          {{/each}}
                        </tbody>
                      </table>
                    </div>
                    <div class="dis-allies">
                      <h3 class="dis-allies-heading">Übereinstimmung mit
                        <PartyChip
                          @code={{this.allyParty}}
                          @party={{this.allyParty}}
                        /></h3>
                      <CanonRows @framed={{true}}>
                        {{#each this.allies key="code" as |a i|}}
                          {{! template-lint-disable no-invalid-interactive }}
                          <div
                            class="dis-bar-row dis-ally"
                            title="{{num a.n}} gemeinsame Abstimmungen"
                            {{on "click" (fn this.toggle a.code)}}
                          >
                            <span class="dis-bar-label"><PartyChip
                                @code={{a.code}}
                                @party={{a.code}}
                              /></span>
                            <CanonBar
                              class="dis-bar"
                              aria-hidden="true"
                              @share={{a.share}}
                            />
                            <span class="dis-bar-value"><span
                                {{rough
                                  "underline"
                                  show=(eq i 0)
                                  color="--dis-highlight"
                                  strokeWidth=2.5
                                  padding=2
                                  delay=800
                                }}
                              >{{pct a.share}}</span></span>
                          </div>
                        {{/each}}
                      </CanonRows>
                    </div>
                  </div>
                </section>
              {{/if}}

              {{#if data.dissenters.length}}
                <section class="dis-stat-section">
                  <div class="dis-stat-headrow">
                    <h2 class="dis-stat-heading"><Icon
                        @name="user-x"
                        @size={{20}}
                      />Abweichler</h2>
                    {{#if this.selected}}
                      <button
                        type="button"
                        class="dis-filter-chip"
                        aria-label="Alle Fraktionen"
                        {{rough
                          "underline"
                          color="--dis-highlight"
                          strokeWidth=2.5
                          padding=3
                        }}
                        {{on "click" this.clear}}
                      >
                        <PartyChip
                          @code={{this.selected}}
                          @party={{this.selected}}
                        />
                        <Icon @name="close" @size={{14}} />
                      </button>
                    {{/if}}
                  </div>
                  <p class="dis-stat-note">
                    Mitglieder, die am häufigsten gegen die Mehrheitslinie ihrer
                    Fraktion gestimmt haben.
                  </p>
                  <div class="dis-dissent-wrap">
                    <table class="dis-dissent">
                      <thead>
                        <tr>
                          {{#each DISSENTER_COLUMNS as |c|}}
                            <th
                              scope="col"
                              class={{if c.num "dis-num"}}
                              aria-sort={{this.ariaSort c.key}}
                            >
                              <button
                                type="button"
                                {{on "click" (fn this.sortBy c.key)}}
                              >{{c.label}}
                                <span
                                  class="dis-sort"
                                  aria-hidden="true"
                                >{{this.sortMark c.key}}</span></button>
                            </th>
                          {{/each}}
                        </tr>
                      </thead>
                      <tbody>
                        {{#each this.dissenters as |d|}}
                          <tr
                            class="{{if (this.isHot d.party) 'dis-hot'}}
                              {{if (this.dim d.party) 'dis-dim'}}"
                          >
                            <td data-label="Name">
                              {{#let (this.personFor d) as |id|}}
                                {{#if id}}
                                  <button
                                    type="button"
                                    class="dis-link"
                                    {{on "click" (fn this.openPerson id)}}
                                  >{{d.name}}</button>
                                {{else}}
                                  {{d.name}}
                                {{/if}}
                              {{/let}}
                            </td>
                            <td data-label="Fraktion">
                              <PartyChip @code={{d.party}} @party={{d.party}} />
                            </td>
                            <td class="dis-num" data-label="Abweichungen"><span
                                {{rough
                                  "circle"
                                  show=(this.isTopDissenter d)
                                  padding=6
                                  delay=1200
                                }}
                              >{{num d.dissents}}</span></td>
                            <td class="dis-num" data-label="Stimmen">{{num
                                d.votes
                              }}</td>
                            <td class="dis-num dis-rate" data-label="Quote">
                              <CanonBar
                                class="dis-rate-bar"
                                aria-hidden="true"
                                @share={{this.rateShare d}}
                              />
                              {{pct d.rate}}
                            </td>
                          </tr>
                        {{else}}
                          <tr>
                            <td colspan="5">
                              <CanonEmptyRow @text="Keine Abweichler" />
                            </td>
                          </tr>
                        {{/each}}
                      </tbody>
                    </table>
                  </div>
                </section>
              {{/if}}

              <p class="dis-stat-note">
                Basis:
                {{num data.totals.with_rollcall}}
                namentliche Abstimmungen. Zuordnung über Nachnamen;
                {{num data.unattributed_entries}}
                Einzelstimmen blieben unzugeordnet und sind nicht enthalten.
              </p>
            {{else}}
              <CanonRows @framed={{true}}>
                <CanonEmptyRow
                  @text="Noch keine namentlichen Abstimmungen ausgewertet — die Statistik füllt sich, sobald Abstimmungspanels (live oder aus Ergebnis-PDFs) erfasst sind."
                />
              </CanonRows>
            {{/if}}
          {{/let}}
        {{/if}}
      </LoadState>
    </CanonPage>
  </template>
}
