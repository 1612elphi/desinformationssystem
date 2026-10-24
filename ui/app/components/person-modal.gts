import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { getPerson } from 'ui/api';
import { InlineLoading } from 'ui/components/loading';
import Modal from 'ui/components/modal';
import PartyChip from 'ui/components/party-chip';
import Load from 'ui/utils/load';
import { VOTE_LABEL } from 'ui/utils/meta';

const voteLabel = (v: string) => VOTE_LABEL[v] || v;
const orDash = (s?: string) => s || '—';

interface Signature {
  Args: { id: string; onClose: () => void };
}

export default class PersonModal extends Component<Signature> {
  @cached
  get load() {
    return new Load(getPerson(this.args.id));
  }

  get person() {
    return this.load.value;
  }

  get summary() {
    const s = this.person?.vote_summary ?? {};
    return {
      ja: s['ja'] || 0,
      nein: s['nein'] || 0,
      enthaltung: s['enthaltung'] || 0,
      abwesend: s['abwesend'] || 0,
    };
  }

  <template>
    <Modal
      @heading={{if this.person.name this.person.name "Person"}}
      @onClose={{@onClose}}
    >
      <div class="dis-detail">
        {{#if this.load.error}}
          <p class="dis-meta-line">Profil konnte nicht geladen werden.</p>
        {{else if this.person}}
          {{#let this.person as |person|}}
            <dl>
              <dt>Fraktion</dt>
              <dd>
                <PartyChip
                  @code={{person.party_code}}
                  @party={{person.party}}
                />
                {{#if person.party}}<span class="dis-meta-line">
                    {{person.party}}</span>{{/if}}
              </dd>
              <dt>Gremien</dt>
              <dd>
                {{#each person.memberships key="body_id" as |m|}}
                  <div>
                    {{m.body_name}}
                    {{#if m.role}}<span class="dis-meta-line">
                        —
                        {{m.role}}</span>{{/if}}
                  </div>
                {{/each}}
              </dd>
            </dl>

            <h4>Abstimmungsverhalten ({{person.votes.length}})</h4>
            {{#if person.votes.length}}
              <div class="dis-vote__tally dis-vote__tally--spaced">
                <span class="dis-vote__n dis-vote__ja">Ja
                  {{this.summary.ja}}</span>
                <span class="dis-vote__n dis-vote__nein">Nein
                  {{this.summary.nein}}</span>
                <span class="dis-vote__n dis-vote__enth">Enthaltung
                  {{this.summary.enthaltung}}</span>
                <span class="dis-vote__n">Abwesend
                  {{this.summary.abwesend}}</span>
              </div>
              <table
                class="cds--data-table cds--data-table--sm cds--data-table--zebra"
              >
                <thead>
                  <tr>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Datum</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Gremium</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >TOP</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Stimme</div></th>
                  </tr>
                </thead>
                <tbody>
                  {{#each person.votes as |v|}}
                    <tr>
                      <td>{{orDash v.meeting_date}}</td>
                      <td>{{orDash v.body_name}}</td>
                      <td>
                        <span
                          class="dis-ellip dis-person-top"
                          title={{if v.agenda_title v.agenda_title v.top_label}}
                        >{{v.top_label}}{{#if v.agenda_title}}
                            —
                            {{v.agenda_title}}{{/if}}</span>
                      </td>
                      <td>
                        <span
                          class="dis-votechip dis-votechip--{{v.vote}}"
                        >{{voteLabel v.vote}}</span>
                      </td>
                    </tr>
                  {{/each}}
                </tbody>
              </table>
              <p class="dis-meta-line dis-spaced-sm">
                Zuordnung über den Nachnamen im Abstimmungspanel; mehrdeutige
                Einträge werden nicht gezählt.
              </p>
            {{else}}
              <p class="dis-meta-line">
                Keine namentlichen Abstimmungen erfasst — Auswertung wächst mit
                jeder Sitzung, deren Abstimmungspanel ausgelesen wird.
              </p>
            {{/if}}
          {{/let}}
        {{else}}
          <InlineLoading @text="Lädt…" />
        {{/if}}
      </div>
    </Modal>
  </template>
}
