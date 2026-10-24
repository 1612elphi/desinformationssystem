import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { htmlSafe } from '@ember/template';
import { pageTitle } from 'ember-page-title';
import CanonBar from 'delphicanon/components/canon-bar';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonPage from 'delphicanon/components/canon-page';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonTiles from 'delphicanon/components/canon-tiles';
import type { Analytics, AnalyticsParty } from 'ui/api';
import DataTable from 'ui/components/data-table';
import { LoadState } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import type Load from 'ui/utils/load';
import { eq, num, pct } from 'ui/utils/meta';
import './statistik.css';

const DISSENTER_COLUMNS = [
  { key: 'name', label: 'Name (Panel)' },
  { key: 'party', label: 'Fraktion', width: '7rem' },
  { key: 'dissents', label: 'Abweichungen', width: '8rem', cls: 'dis-num' },
  { key: 'votes', label: 'Stimmen', width: '6rem', cls: 'dis-num' },
];

const basis = (n: number | null | undefined) =>
  htmlSafe(`flex-basis: ${(n ?? 0) * 100}%`);

interface Signature {
  Args: { model: Load<Analytics> };
}

export default class StatistikTemplate extends Component<Signature> {
  get data() {
    return this.args.model.value;
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

  get maxCast() {
    return Math.max(
      1,
      ...(this.data?.parties ?? []).map((p) => p.ja + p.nein + p.enthaltung),
    );
  }

  // stacked bar segments share one scale: the party with the most votes fills the track
  segment = (n: number) => basis(n / this.maxCast);

  cohesion = (p: AnalyticsParty) => p.cohesion ?? 0;

  get codes() {
    return (this.data?.parties ?? []).map((p) => p.code);
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
        `background: color-mix(in srgb, var(--canon-primary) ${Math.round(5 + c.agree * 50)}%, var(--canon-card))`,
      ),
    };
  };

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
                <h2 class="dis-stat-heading">Fraktionsdisziplin</h2>
                <p class="dis-stat-note">
                  Anteil der abgegebenen Stimmen, die der Mehrheitslinie der
                  eigenen Fraktion folgen.
                </p>
                <CanonRows @framed={{true}}>
                  {{#each data.parties key="code" as |p|}}
                    <div class="dis-bar-row">
                      <span class="dis-bar-label"><PartyChip
                          @code={{p.code}}
                          @party={{p.code}}
                        /></span>
                      <CanonBar
                        class="dis-bar"
                        aria-hidden="true"
                        @share={{this.cohesion p}}
                      />
                      <span class="dis-bar-value">{{pct p.cohesion}}</span>
                      <span class="dis-bar-meta">{{num p.votes}}
                        Abst. ·
                        {{num p.members_cast}}
                        Stimmen</span>
                    </div>
                  {{/each}}
                </CanonRows>
              </section>

              <section class="dis-stat-section">
                <h2 class="dis-stat-heading">Stimmverteilung</h2>
                <p class="dis-stat-note">
                  Alle zugeordneten Einzelstimmen je Fraktion (Gelb = Ja, Rot =
                  Nein, Grau = Enthaltung — wie auf dem Abstimmungspanel).
                </p>
                <CanonRows @framed={{true}}>
                  {{#each data.parties key="code" as |p|}}
                    <div class="dis-bar-row">
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
                  <h2 class="dis-stat-heading">Übereinstimmung zwischen
                    Fraktionen</h2>
                  <p class="dis-stat-note">
                    Wie oft zwei Fraktionen bei derselben Abstimmung dieselbe
                    Mehrheitslinie hatten.
                  </p>
                  <div class="dis-matrix-wrap">
                    <table class="dis-matrix">
                      <thead>
                        <tr>
                          <td></td>
                          {{#each this.codes as |code|}}
                            <th scope="col">{{code}}</th>
                          {{/each}}
                        </tr>
                      </thead>
                      <tbody>
                        {{#each this.codes as |row|}}
                          <tr>
                            <th scope="row">{{row}}</th>
                            {{#each this.codes as |col|}}
                              {{#if (eq row col)}}
                                <td class="dis-matrix-self"></td>
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
                </section>
              {{/if}}

              {{#if data.dissenters.length}}
                <section class="dis-stat-section">
                  <h2 class="dis-stat-heading">Abweichler</h2>
                  <p class="dis-stat-note">
                    Mitglieder, die am häufigsten gegen die Mehrheitslinie ihrer
                    Fraktion gestimmt haben.
                  </p>
                  <DataTable
                    @columns={{DISSENTER_COLUMNS}}
                    @rows={{data.dissenters}}
                    as |d col|
                  >
                    {{#if (eq col.key "name")}}
                      {{d.name}}
                    {{else if (eq col.key "party")}}
                      <PartyChip @code={{d.party}} @party={{d.party}} />
                    {{else if (eq col.key "dissents")}}
                      {{num d.dissents}}
                    {{else}}
                      {{num d.votes}}
                    {{/if}}
                  </DataTable>
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
