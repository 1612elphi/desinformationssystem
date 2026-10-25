import Component from '@glimmer/component';
import { cached, tracked } from '@glimmer/tracking';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { getOwner } from '@ember/owner';
import { service } from '@ember/service';
import { htmlSafe } from '@ember/template';
import type RouterService from '@ember/routing/router-service';
import CanonButton from 'delphicanon/components/canon-button';
import CanonTiles from 'delphicanon/components/canon-tiles';
import 'delphicanon/components/canon-heading.css';
import { getPerson } from 'ui/api';
import Avatar from 'ui/components/avatar';
import type GremienController from 'ui/controllers/gremien';
import DataTable, { type Column } from 'ui/components/data-table';
import Icon from 'ui/components/icon';
import { LoadError, LoadState } from 'ui/components/loading';
import Modal, { type PaneContext } from 'ui/components/modal';
import PartyChip from 'ui/components/party-chip';
import PersonTimeline from 'ui/components/person-timeline';
import Tag from 'ui/components/tag';
import VoteBar from 'ui/components/vote-bar';
import rough from 'ui/modifiers/rough';
import shortcut from 'ui/modifiers/shortcut';
import type NavService from 'ui/services/nav';
import {
  againstParty,
  deDate,
  isChair,
  partyMark,
  roleTitle,
  type PartyVote,
  type PersonDetail,
} from 'ui/utils/api-menschen';
import Load from 'ui/utils/load';
import { SUBMITTERS, VOTE_LABEL, eq, num, pct } from 'ui/utils/meta';
import './person-modal.css';

const voteLabel = (v: string) => VOTE_LABEL[v] || v;
const orDash = (s?: string) => s || '—';
const voteClass = (v?: string) => (v ?? '').replace(/[^a-z]/g, '');
const markStyle = (code?: string) =>
  htmlSafe(`--dis-person-mark: ${partyMark(code)}`);
const tallyOf = (v: PartyVote, k: string) => v.party_tally?.[k] ?? 0;

const VOTE_COLUMNS: Column[] = [
  { key: 'date', label: 'Datum', width: '7rem' },
  { key: 'body', label: 'Gremium', width: '30%' },
  { key: 'top', label: 'TOP' },
  { key: 'vote', label: 'Stimme', width: '7rem' },
];

const SHORT_LIST = 8;
const longer = (xs: unknown[], n: number) => xs.length > n;

interface Signature {
  Args: { id: string; onClose: () => void; pane?: PaneContext };
}

export default class PersonModal extends Component<Signature> {
  @service declare nav: NavService;
  @service declare router: RouterService;

  @tracked showAllAgainst = false;
  @tracked tableOpen = false;

  @cached
  get load() {
    return this.args.id
      ? new Load<PersonDetail>(getPerson(this.args.id))
      : undefined;
  }

  get person() {
    return this.load?.value;
  }

  get votes(): PartyVote[] {
    return this.person?.votes ?? [];
  }

  @cached
  get against() {
    return this.votes.filter(againstParty);
  }

  get shownAgainst() {
    return this.showAllAgainst
      ? this.against
      : this.against.slice(0, SHORT_LIST);
  }

  get partyName() {
    const p = this.person;
    return (p?.party_code && SUBMITTERS[p.party_code]?.name) || p?.party || '';
  }

  get roles() {
    const titles = new Set<string>();
    for (const m of this.person?.memberships ?? [])
      for (const t of roleTitle(m.role).split(/\s*[,|]\s*/))
        if (t) titles.add(t);
    return [...titles].join(' · ');
  }

  get summary() {
    const s = this.person?.vote_summary ?? {};
    const cast = (s['ja'] ?? 0) + (s['nein'] ?? 0) + (s['enthaltung'] ?? 0);
    const n = (k: string) => s[k] ?? 0;
    return {
      ja: n('ja'),
      nein: n('nein'),
      enthaltung: n('enthaltung'),
      cast,
      absent: n('abwesend'),
    };
  }

  get tiles() {
    const { cast, absent } = this.summary;
    const lined = this.votes.filter(
      (v) => v.party_line && v.vote !== 'abwesend' && v.vote,
    ).length;
    const attendance = cast + absent ? cast / (cast + absent) : undefined;
    const loyalty = lined ? 1 - this.against.length / lined : undefined;
    return [
      { label: 'Abstimmungen', value: num(this.votes.length) },
      { label: 'Anwesenheit', value: pct(attendance), level: attendance },
      { label: 'Mit Fraktion', value: pct(loyalty), level: loyalty },
      { label: 'Gegen Fraktion', value: num(this.against.length) },
    ];
  }

  get roster() {
    if (this.router.currentRouteName !== 'gremien') return undefined;
    const c = getOwner(this)?.lookup('controller:gremien') as
      GremienController | undefined;
    const members = c?.currentId ? c.members : [];
    const i = members.findIndex((m) => m.person_id === this.args.id);
    if (i < 0) return undefined;
    return {
      pos: `${num(i + 1)} / ${num(members.length)}`,
      prev: members[i - 1]?.person_id,
      next: members[i + 1]?.person_id,
    };
  }

  step = (id?: string) => {
    if (!id) return;
    this.showAllAgainst = false;
    this.tableOpen = false;
    this.nav.replaceTop('person', id);
  };

  stepPrev = () => this.step(this.roster?.prev);
  stepNext = () => this.step(this.roster?.next);

  openMeeting = (id: string) => this.nav.open({ meeting: id });
  openBody = (id: string) => this.nav.go('gremien', { gremium: id });
  toggleAgainst = () => (this.showAllAgainst = !this.showAllAgainst);
  onToggleTable = (e: Event) =>
    (this.tableOpen = (e.target as HTMLDetailsElement).open);

  <template>
    <Modal
      @heading={{if this.person.name this.person.name "Person"}}
      @pane={{@pane}}
      @onClose={{@onClose}}
    >
      {{#if this.load.error}}
        <LoadError @error="Profil konnte nicht geladen werden." />
      {{else}}
        <LoadState @load={{this.load}}>
          {{#let this.person as |person|}}
            <div
              class="dis-person"
              style={{markStyle person.party_code}}
              {{shortcut "[" this.stepPrev}}
              {{shortcut "]" this.stepNext}}
            >
              <header class="dis-person-head">
                <Avatar
                  class="dis-person-avatar"
                  @personId={{person.person_id}}
                  @name={{person.name}}
                />
                <div class="dis-person-id">
                  <div class="dis-person-party">
                    <PartyChip
                      @code={{person.party_code}}
                      @party={{person.party}}
                    />
                    {{#if this.partyName}}<span>{{this.partyName}}</span>{{/if}}
                  </div>
                  {{#if this.roles}}
                    <div class="dis-person-muted">{{this.roles}}</div>
                  {{/if}}
                  <div class="dis-tags dis-person-bodies">
                    {{#each person.memberships key="body_id" as |m|}}
                      <Tag
                        class={{if (isChair m.role) "dis-person-chair"}}
                        title={{m.role}}
                        @onClick={{fn this.openBody m.body_id}}
                      >{{#if (isChair m.role)}}<Icon
                            @name="crown"
                            @size={{12}}
                          />
                        {{/if}}{{m.body_name}}</Tag>
                    {{/each}}
                  </div>
                </div>
                {{#if this.roster}}
                  <div class="dis-person-step">
                    <CanonButton
                      @kind="cell"
                      @icon={{component Icon name="chevron-left"}}
                      @label="Vorherige Person"
                      @disabled={{if this.roster.prev false true}}
                      aria-keyshortcuts="["
                      @onClick={{this.stepPrev}}
                    />
                    <span class="dis-person-muted">{{this.roster.pos}}</span>
                    <CanonButton
                      @kind="cell"
                      @icon={{component Icon name="chevron-right"}}
                      @label="Nächste Person"
                      @disabled={{if this.roster.next false true}}
                      aria-keyshortcuts="]"
                      @onClick={{this.stepNext}}
                    />
                  </div>
                {{/if}}
              </header>

              {{#if this.votes.length}}
                <CanonTiles @items={{this.tiles}} />
                <div class="dis-person-bar">
                  <VoteBar
                    @ja={{this.summary.ja}}
                    @nein={{this.summary.nein}}
                    @enthaltung={{this.summary.enthaltung}}
                  />
                  <span class="dis-person-muted">Abwesend
                    {{num this.summary.absent}}</span>
                </div>

                <h4 class="canon-heading dis-person-heading"><Icon
                    @name="split"
                    @size={{14}}
                  />Gegen die Fraktion
                  <span
                    {{rough
                      "circle"
                      show=(if this.against.length true false)
                      padding=5
                      delay=600
                    }}
                  >{{num this.against.length}}</span></h4>
                <div class="dis-person-list">
                  {{#each this.shownAgainst as |v|}}
                    <button
                      type="button"
                      class="dis-person-against"
                      {{on "click" (fn this.openMeeting v.meeting_id)}}
                    >
                      <span class="dis-person-against-when">
                        {{deDate v.meeting_date}}
                        <span class="dis-person-muted">{{v.body_name}}</span>
                      </span>
                      <span class="dis-person-against-top">{{v.top_label}}{{#if
                          v.agenda_title
                        }}
                          —
                          {{v.agenda_title}}{{/if}}</span>
                      <span class="dis-person-against-votes">
                        <span
                          class="dis-person-vote dis-person-vote--{{voteClass
                              v.vote
                            }}"
                        >{{voteLabel v.vote}}</span>
                        <span class="dis-person-muted">≠
                          {{v.party_code}}</span>
                        <span
                          class="dis-person-vote dis-person-vote--{{voteClass
                              v.party_line
                            }}"
                        >{{voteLabel (orDash v.party_line)}}</span>
                        <VoteBar
                          @compact={{true}}
                          @ja={{tallyOf v "ja"}}
                          @nein={{tallyOf v "nein"}}
                          @enthaltung={{tallyOf v "enthaltung"}}
                        />
                      </span>
                    </button>
                  {{else}}
                    <p class="dis-person-muted dis-person-pad">—</p>
                  {{/each}}
                  {{#if (longer this.against SHORT_LIST)}}
                    <CanonButton
                      class="dis-person-more"
                      @kind="cell"
                      @text={{if
                        this.showAllAgainst
                        "Weniger anzeigen"
                        "Alle anzeigen"
                      }}
                      @onClick={{this.toggleAgainst}}
                    />
                  {{/if}}
                </div>

                <h4 class="canon-heading dis-person-heading"><Icon
                    @name="history"
                    @size={{14}}
                  />Verlauf</h4>
                <PersonTimeline
                  @votes={{this.votes}}
                  @onOpen={{this.openMeeting}}
                />

                <details
                  class="dis-person-all"
                  {{on "toggle" this.onToggleTable}}
                >
                  <summary><Icon @name="table" @size={{14}} />
                    Alle Abstimmungen ({{num this.votes.length}})</summary>
                  {{#if this.tableOpen}}
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
                  {{/if}}
                </details>
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
