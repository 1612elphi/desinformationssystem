import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import CanonTiles from 'delphicanon/components/canon-tiles';
import 'delphicanon/components/canon-heading.css';
import { getPerson, type PersonVote } from 'ui/api';
import DataTable, { type Column } from 'ui/components/data-table';
import { LoadError, LoadState } from 'ui/components/loading';
import Modal from 'ui/components/modal';
import PartyChip from 'ui/components/party-chip';
import Load from 'ui/utils/load';
import { VOTE_LABEL, num } from 'ui/utils/meta';
import './person-modal.css';

const voteLabel = (v: string) => VOTE_LABEL[v] || v;
const orDash = (s?: string) => s || '—';
const eq = (a: string, b: string) => a === b;
const voteClass = (v: string) => v.replace(/[^a-z]/g, '');

const VOTE_COLUMNS: Column[] = [
  { key: 'date', label: 'Datum', width: '7rem' },
  { key: 'body', label: 'Gremium', width: '30%' },
  { key: 'top', label: 'TOP' },
  { key: 'vote', label: 'Stimme', width: '7rem' },
];

interface Signature {
  Args: { id: string; open: boolean; onClose: () => void };
}

export default class PersonModal extends Component<Signature> {
  @cached
  get load() {
    return this.args.id ? new Load(getPerson(this.args.id)) : undefined;
  }

  get person() {
    return this.load?.value;
  }

  get votes(): PersonVote[] {
    return this.person?.votes ?? [];
  }

  get tiles() {
    const s = this.person?.vote_summary ?? {};
    return ['ja', 'nein', 'enthaltung', 'abwesend'].map((k) => ({
      label: voteLabel(k),
      value: num(s[k]),
    }));
  }

  <template>
    <Modal
      @heading={{if this.person.name this.person.name "Person"}}
      @open={{@open}}
      @onClose={{@onClose}}
    >
      {{#if this.load.error}}
        <LoadError @error="Profil konnte nicht geladen werden." />
      {{else}}
        <LoadState @load={{this.load}}>
          {{#let this.person as |person|}}
            <div class="dis-person">
              <dl class="dis-person-facts">
                <dt>Fraktion</dt>
                <dd>
                  <PartyChip
                    @code={{person.party_code}}
                    @party={{person.party}}
                  />
                  {{#if person.party}}<span class="dis-person-muted">
                      {{person.party}}</span>{{/if}}
                </dd>
                <dt>Gremien</dt>
                <dd>
                  {{#each person.memberships key="body_id" as |m|}}
                    <div>
                      {{m.body_name}}
                      {{#if m.role}}<span class="dis-person-muted">
                          —
                          {{m.role}}</span>{{/if}}
                    </div>
                  {{/each}}
                </dd>
              </dl>

              <h4 class="canon-heading">Abstimmungsverhalten ({{num
                  this.votes.length
                }})</h4>
              {{#if this.votes.length}}
                <CanonTiles @items={{this.tiles}} />
                <DataTable
                  @columns={{VOTE_COLUMNS}}
                  @rows={{this.votes}}
                  as |v col|
                >
                  {{#if (eq col.key "date")}}
                    {{orDash v.meeting_date}}
                  {{else if (eq col.key "body")}}
                    {{orDash v.body_name}}
                  {{else if (eq col.key "top")}}
                    <span>{{v.top_label}}{{#if v.agenda_title}}
                        —
                        {{v.agenda_title}}{{/if}}</span>
                  {{else}}
                    <span
                      class="dis-person-vote dis-person-vote--{{voteClass
                          v.vote
                        }}"
                    >{{voteLabel v.vote}}</span>
                  {{/if}}
                </DataTable>
                <p class="dis-person-muted">
                  Zuordnung über den Nachnamen im Abstimmungspanel; mehrdeutige
                  Einträge werden nicht gezählt.
                </p>
              {{else}}
                <p class="dis-person-muted">
                  Keine namentlichen Abstimmungen erfasst — Auswertung wächst
                  mit jeder Sitzung, deren Abstimmungspanel ausgelesen wird.
                </p>
              {{/if}}
            </div>
          {{/let}}
        </LoadState>
      {{/if}}
    </Modal>
  </template>
}
