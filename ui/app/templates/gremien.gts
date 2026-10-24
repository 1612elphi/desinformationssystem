import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import type GremienController from 'ui/controllers/gremien';
import type NavService from 'ui/services/nav';
import { InlineLoading } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import { eq } from 'ui/utils/meta';

interface Signature {
  Args: { controller: GremienController };
}

export default class GremienTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get list() {
    return this.args.controller.list.value ?? [];
  }

  openPerson = (id: string) => this.nav.open({ person: id });

  <template>
    {{#let @controller as |c|}}
      <div class="dis-committees">
        <div class="dis-committee-list">
          <h4>Gremien ({{this.list.length}})</h4>
          {{#each this.list key="id" as |g|}}
            <button
              type="button"
              class="dis-committee-item
                {{if (eq c.selectedId g.id) 'is-selected'}}"
              {{on "click" (fn c.select g.id)}}
            >
              <div>{{g.name}}</div>
              <div class="dis-meta-line">
                {{g.members}}
                Mitglieder ·
                {{g.meetings}}
                Sitzungen ·
                {{g.documents}}
                Dok.
              </div>
            </button>
          {{/each}}
        </div>
        <div class="dis-detail-panel dis-detail">
          {{#let c.selected.value as |sel|}}
            {{#if c.selected.pending}}
              <InlineLoading @text="Lädt…" />
            {{else if sel}}
              <h3>{{sel.name}}</h3>
              {{#if sel.composition.length}}
                <h4>Zusammensetzung</h4>
                <div class="dis-tags">
                  {{#each sel.composition as |p|}}
                    <span class="dis-composition">
                      <PartyChip @code={{p.code}} @party={{p.code}} />
                      <span class="dis-meta-line">{{p.n}}</span>
                    </span>
                  {{/each}}
                </div>
              {{/if}}
              <h4>Mitglieder ({{sel.members.length}})</h4>
              <table
                class="cds--data-table cds--data-table--sm cds--data-table--zebra"
              >
                <thead>
                  <tr>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Name</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Fraktion</div></th>
                    <th scope="col"><div
                        class="cds--table-header-label"
                      >Funktion</div></th>
                  </tr>
                </thead>
                <tbody>
                  {{#each sel.members key="person_id" as |m|}}
                    <tr
                      class="dis-row-clickable"
                      title="Profil und Abstimmungsverhalten anzeigen"
                      {{on "click" (fn this.openPerson m.person_id)}}
                    >
                      <td>{{m.name}}</td>
                      <td><PartyChip
                          @code={{m.party_code}}
                          @party={{m.party}}
                        /></td>
                      <td>{{m.role}}</td>
                    </tr>
                  {{/each}}
                </tbody>
              </table>
            {{else}}
              <div class="dis-empty dis-empty--bare">
                Gremium auswählen, um Mitglieder und Zusammensetzung zu sehen.
              </div>
            {{/if}}
          {{/let}}
        </div>
      </div>
    {{/let}}
  </template>
}
