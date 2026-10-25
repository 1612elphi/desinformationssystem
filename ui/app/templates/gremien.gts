import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import { pageTitle } from 'ember-page-title';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonSplit from 'delphicanon/components/canon-split';
import 'delphicanon/components/canon-heading.css';
import type { Member } from 'ui/api';
import type GremienController from 'ui/controllers/gremien';
import type NavService from 'ui/services/nav';
import DataTable, { type Column } from 'ui/components/data-table';
import { LoadState } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import { eq, num } from 'ui/utils/meta';
import './gremien.css';

const MEMBER_COLUMNS: Column[] = [
  { key: 'name', label: 'Name' },
  { key: 'party', label: 'Fraktion', width: '8rem' },
  { key: 'role', label: 'Funktion' },
];

const TITLE_ID = 'dis-gremium-title';
const memberLabel = (m: Member) => m.name;

interface Signature {
  Args: { controller: GremienController };
}

export default class GremienTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get list() {
    return this.args.controller.list.value ?? [];
  }

  get selectedName() {
    const c = this.args.controller;
    return (
      this.list.find((g) => g.id === c.selectedId)?.name ??
      c.selected?.value?.name ??
      ''
    );
  }

  get members() {
    return this.args.controller.selected?.value?.members ?? [];
  }

  // in one column the detail sits below the list, out of sight
  pick = (id: string) => {
    this.args.controller.select(id);
    requestAnimationFrame(() => {
      const title = document.getElementById(TITLE_ID);
      if (!title) return;
      const r = title.getBoundingClientRect();
      if (r.top >= 0 && r.bottom <= window.innerHeight) return;
      title.scrollIntoView({ block: 'start' });
      title.focus({ preventScroll: true });
    });
  };

  openPerson = (m: Member) => this.nav.open({ person: m.person_id });

  <template>
    {{pageTitle "Gremien"}}
    {{#let @controller as |c|}}
      <CanonSplit class="dis-gremien" @sidebarWidth={{380}}>
        <:sidebar>
          <CanonRows @framed={{true}}>
            <h2 class="canon-heading">Gremien
              {{#if c.list.value}}({{num this.list.length}}){{/if}}</h2>
            <LoadState @load={{c.list}}>
              {{#each this.list key="id" as |g|}}
                <button
                  type="button"
                  class="dis-gremien-item"
                  aria-current={{if (eq c.selectedId g.id) "true"}}
                  {{on "click" (fn this.pick g.id)}}
                >
                  <span>{{g.name}}</span>
                  <span class="dis-gremien-meta">
                    {{num g.members}}
                    Mitglieder ·
                    {{num g.meetings}}
                    Sitzungen ·
                    {{num g.documents}}
                    Dok.
                  </span>
                </button>
              {{/each}}
            </LoadState>
          </CanonRows>
        </:sidebar>
        <:main>
          {{#if c.selectedId}}
            <CanonRows @framed={{true}}>
              <h2 id={{TITLE_ID}} class="canon-heading" tabindex="-1">{{if
                  this.selectedName
                  this.selectedName
                  "Gremium"
                }}</h2>
              <LoadState @load={{c.selected}}>
                {{#let c.selected.value as |sel|}}
                  {{#if sel.composition.length}}
                    <h3 class="canon-heading">Zusammensetzung</h3>
                    <div class="dis-gremien-composition">
                      {{#each sel.composition as |p|}}
                        <span class="dis-gremien-party">
                          <PartyChip @code={{p.code}} @party={{p.code}} />
                          <span class="dis-gremien-meta">{{num p.n}}</span>
                        </span>
                      {{/each}}
                    </div>
                  {{/if}}
                  <h3 class="canon-heading">Mitglieder ({{num
                      this.members.length
                    }})</h3>
                  <DataTable
                    class="dis-gremien-members"
                    title="Profil und Abstimmungsverhalten anzeigen"
                    @columns={{MEMBER_COLUMNS}}
                    @rows={{this.members}}
                    @onOpen={{this.openPerson}}
                    @rowLabel={{memberLabel}}
                    as |m col|
                  >
                    {{#if (eq col.key "name")}}
                      {{m.name}}
                    {{else if (eq col.key "party")}}
                      <PartyChip @code={{m.party_code}} @party={{m.party}} />
                    {{else}}
                      {{m.role}}
                    {{/if}}
                  </DataTable>
                {{/let}}
              </LoadState>
            </CanonRows>
          {{else}}
            <CanonRows @framed={{true}}>
              <CanonEmptyRow
                @text="Gremium auswählen, um Mitglieder und Zusammensetzung zu sehen."
              />
            </CanonRows>
          {{/if}}
        </:main>
      </CanonSplit>
    {{/let}}
  </template>
}
