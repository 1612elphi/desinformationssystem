import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { htmlSafe } from '@ember/template';
import type { Analytics, AnalyticsParty } from 'ui/api';
import { TableSkeleton } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import type Load from 'ui/utils/load';
import { eq, num, pct } from 'ui/utils/meta';

const width = (fraction: number) => htmlSafe(`width: ${fraction * 100}%`);

interface Signature {
  Args: { model: Load<Analytics> };
}

export default class StatistikTemplate extends Component<Signature> {
  get data() {
    return this.args.model.value;
  }

  get failed() {
    return !!this.args.model.error;
  }

  share = (part: number, whole: number) => (whole ? pct(part / whole) : '—');

  cohesionWidth = (p: AnalyticsParty) =>
    htmlSafe(`width: ${Math.round((p.cohesion || 0) * 100)}%`);

  get maxCast() {
    return Math.max(
      1,
      ...(this.data?.parties ?? []).map((p) => p.members_cast),
    );
  }

  // Stacked bar: segment share of this party's votes, scaled to the largest party.
  segment = (p: AnalyticsParty, n: number) => {
    const cast = p.ja + p.nein + p.enthaltung;
    return width((cast ? n / cast : 0) * (cast / this.maxCast));
  };

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
      title: `${row} & ${col}: ${pct(c.agree)} (${c.n} gemeinsame Abstimmungen)`,
      text: String(Math.round(c.agree * 100)),
      style: htmlSafe(
        `background: rgba(69, 137, 255, ${0.08 + c.agree * 0.55})`,
      ),
    };
  };

  <template>
    {{#if this.failed}}
      <div class="dis-empty">Statistik konnte nicht geladen werden.</div>
    {{else if this.data}}
      {{#let this.data.totals as |t|}}
        <div class="dis-statsview">
          <div class="dis-stats">
            <div class="dis-stat"><div class="num">{{num t.votes}}</div><div
                class="lbl"
              >Abstimmungen</div></div>
            <div class="dis-stat"><div class="num">{{num
                  t.with_rollcall
                }}</div><div class="lbl">Namentlich</div></div>
            <div class="dis-stat"><div class="num">{{this.share
                  t.unanimous
                  t.votes
                }}</div><div class="lbl">Einstimmig</div></div>
            <div class="dis-stat"><div class="num">{{this.share
                  t.contested
                  t.votes
                }}</div><div class="lbl">Mit Gegenstimmen</div></div>
          </div>

          {{#if this.data.parties.length}}
            <h4>Fraktionsdisziplin</h4>
            <p class="dis-meta-line">
              Anteil der abgegebenen Stimmen, die der Mehrheitslinie der eigenen
              Fraktion folgen.
            </p>
            <div class="dis-bars">
              {{#each this.data.parties key="code" as |p|}}
                <div class="dis-bar-row">
                  <span class="dis-bar-label"><PartyChip
                      @code={{p.code}}
                      @party={{p.code}}
                    /></span>
                  <span class="dis-bar-track" aria-hidden="true">
                    <span
                      class="dis-bar-fill"
                      style={{this.cohesionWidth p}}
                    ></span>
                  </span>
                  <span class="dis-bar-value">{{pct p.cohesion}}</span>
                  <span class="dis-meta-line">{{p.votes}}
                    Abst. ·
                    {{p.members_cast}}
                    Stimmen</span>
                </div>
              {{/each}}
            </div>

            <h4>Stimmverteilung</h4>
            <p class="dis-meta-line">
              Alle zugeordneten Einzelstimmen je Fraktion (Gelb = Ja, Rot =
              Nein, Grau = Enthaltung — wie auf dem Abstimmungspanel).
            </p>
            <div class="dis-bars">
              {{#each this.data.parties key="code" as |p|}}
                <div class="dis-bar-row">
                  <span class="dis-bar-label"><PartyChip
                      @code={{p.code}}
                      @party={{p.code}}
                    /></span>
                  <span
                    class="dis-bar-track dis-bar-track--stacked"
                    aria-hidden="true"
                  >
                    <span
                      class="dis-seg dis-seg--ja"
                      style={{this.segment p p.ja}}
                    ></span>
                    <span
                      class="dis-seg dis-seg--nein"
                      style={{this.segment p p.nein}}
                    ></span>
                    <span
                      class="dis-seg dis-seg--enth"
                      style={{this.segment p p.enthaltung}}
                    ></span>
                  </span>
                  <span class="dis-bar-value dis-votetext">
                    <span class="dis-vote__ja">Ja {{p.ja}}</span>
                    ·
                    <span class="dis-vote__nein">Nein {{p.nein}}</span>
                    ·
                    <span class="dis-vote__enth">Enth. {{p.enthaltung}}</span>
                  </span>
                </div>
              {{/each}}
            </div>

            {{#if this.showMatrix}}
              <h4>Übereinstimmung zwischen Fraktionen</h4>
              <p class="dis-meta-line">
                Wie oft zwei Fraktionen bei derselben Abstimmung dieselbe
                Mehrheitslinie hatten.
              </p>
              <div class="dis-matrix-wrap">
                <table class="dis-matrix">
                  <thead>
                    <tr>
                      <th></th>
                      {{#each this.codes as |code|}}<th>{{code}}</th>{{/each}}
                    </tr>
                  </thead>
                  <tbody>
                    {{#each this.codes as |row|}}
                      <tr>
                        <th>{{row}}</th>
                        {{#each this.codes as |col|}}
                          {{#if (eq row col)}}
                            <td class="dis-matrix__self"></td>
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
            {{/if}}

            {{#if this.data.dissenters.length}}
              <h4>Abweichler</h4>
              <p class="dis-meta-line">
                Mitglieder, die am häufigsten gegen die Mehrheitslinie ihrer
                Fraktion gestimmt haben.
              </p>
              <table
                class="cds--data-table cds--data-table--sm cds--data-table--zebra dis-dissenters"
              >
                <thead>
                  <tr>
                    <th scope="col"><div class="cds--table-header-label">Name
                        (Panel)</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Fraktion</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Abweichungen</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Stimmen</div></th>
                  </tr>
                </thead>
                <tbody>
                  {{#each this.data.dissenters key="name" as |d|}}
                    <tr>
                      <td>{{d.name}}</td>
                      <td><PartyChip
                          @code={{d.party}}
                          @party={{d.party}}
                        /></td>
                      <td>{{d.dissents}}</td>
                      <td>{{d.votes}}</td>
                    </tr>
                  {{/each}}
                </tbody>
              </table>
            {{/if}}

            <p class="dis-meta-line dis-spaced">
              Basis:
              {{t.with_rollcall}}
              namentliche Abstimmungen. Zuordnung über Nachnamen;
              {{this.data.unattributed_entries}}
              Einzelstimmen blieben unzugeordnet und sind nicht enthalten.
            </p>
          {{else}}
            <div class="dis-empty">
              Noch keine namentlichen Abstimmungen ausgewertet — die Statistik
              füllt sich, sobald Abstimmungspanels (live oder aus Ergebnis-PDFs)
              erfasst sind.
            </div>
          {{/if}}
        </div>
      {{/let}}
    {{else}}
      <TableSkeleton @columns={{4}} @rows={{5}} />
    {{/if}}
  </template>
}
