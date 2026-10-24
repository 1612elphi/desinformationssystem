import Component from '@glimmer/component';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import type SitzungenController from 'ui/controllers/sitzungen';
import { MEETINGS_PAGE } from 'ui/controllers/sitzungen';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import { SelectField } from 'ui/components/fields';
import Icon from 'ui/components/icon';
import { TableSkeleton } from 'ui/components/loading';
import Pagination from 'ui/components/pagination';
import Tag from 'ui/components/tag';
import { num, publicLabel } from 'ui/utils/meta';

interface Signature {
  Args: { model: ApplicationModel; controller: SitzungenController };
}

export default class SitzungenTemplate extends Component<Signature> {
  @service declare nav: NavService;

  get committeeItems() {
    return [
      { id: '', label: 'Alle Gremien' },
      ...(this.args.model.facets.value?.committees ?? []).map((c) => ({
        id: c.name,
        label: c.name,
      })),
    ];
  }

  get meetings() {
    return this.args.controller.meetings.value?.meetings ?? [];
  }

  get total() {
    return this.args.controller.meetings.value?.total ?? 0;
  }

  get paged() {
    return this.total > MEETINGS_PAGE;
  }

  toggleUpcoming = () =>
    this.args.controller.setUpcoming(!this.args.controller.upcoming);

  openMeeting = (id: string) => this.nav.open({ meeting: id });

  <template>
    {{#let @controller as |c|}}
      <div class="dis-panel">
        <div class="dis-filters dis-filters--meetings">
          <div class="cds--form-item">
            <div class="cds--toggle">
              <button
                id="upcoming"
                type="button"
                role="switch"
                class="cds--toggle__button"
                aria-checked={{if c.upcoming "true" "false"}}
                {{on "click" this.toggleUpcoming}}
              ></button>
              <label for="upcoming" class="cds--toggle__label">
                <span class="cds--toggle__label-text">Zeitraum</span>
                <div
                  class="cds--toggle__appearance cds--toggle__appearance--sm"
                >
                  <div
                    class="cds--toggle__switch
                      {{if c.upcoming 'cds--toggle__switch--checked'}}"
                  >
                    <svg
                      class="cds--toggle__check"
                      width="6"
                      height="5"
                      viewBox="0 0 6 5"
                      aria-hidden="true"
                    >
                      <path d="M2.2 2.7L5 0 6 1 2.2 5 0 2.7 1 1.5z" />
                    </svg>
                  </div>
                  <span class="cds--toggle__text" aria-hidden="true">
                    {{if c.upcoming "Nur kommende" "Alle Sitzungen"}}
                  </span>
                </div>
              </label>
            </div>
          </div>
          <SelectField
            @id="m-committee"
            @label="Gremium"
            @items={{this.committeeItems}}
            @value={{c.committee}}
            @onChange={{c.setCommittee}}
          />
        </div>
        <p class="dis-meta-line dis-ics">
          <a
            class="dis-feed-link"
            href={{c.icsUrl}}
            title="Sitzungen als Kalender abonnieren (ICS)"
          >
            <Icon @name="calendar" @size={{14}} />
            Kalender abonnieren (ICS)
          </a>
        </p>
      </div>

      {{#if c.meetings.pending}}
        <TableSkeleton @columns={{6}} @rows={{6}} />
      {{else if this.meetings.length}}
        <div class="cds--data-table-container">
          <div class="cds--data-table-header">
            <h4 class="cds--data-table-header__title">
              {{if c.upcoming "Kommende Sitzungen" "Alle Sitzungen"}}
            </h4>
            <p class="cds--data-table-header__description">{{num this.total}}
              Sitzung(en)</p>
          </div>
          <div class="cds--data-table-content">
            <table
              class="cds--data-table cds--data-table--lg cds--data-table--zebra"
            >
              <thead>
                <tr>
                  <th scope="col"><div
                      class="cds--table-header-label"
                    >Datum</div></th>
                  <th scope="col"><div
                      class="cds--table-header-label"
                    >Zeit</div></th>
                  <th scope="col"><div
                      class="cds--table-header-label"
                    >Gremium</div></th>
                  <th scope="col"><div
                      class="cds--table-header-label"
                    >Öffentlich</div></th>
                  <th scope="col"><div
                      class="cds--table-header-label"
                    >TOPs</div></th>
                  <th scope="col"><div
                      class="cds--table-header-label"
                    >Dok.</div></th>
                </tr>
              </thead>
              <tbody>
                {{#each this.meetings key="id" as |m|}}
                  <tr
                    class="dis-row-clickable"
                    {{on "click" (fn this.openMeeting m.id)}}
                  >
                    <td>{{if m.date m.date "—"}}</td>
                    <td>{{if m.time m.time "—"}}</td>
                    <td>{{if m.body_name m.body_name "—"}}</td>
                    <td>
                      {{#let (publicLabel m.public) as |pl|}}
                        <Tag @type={{pl.type}} @size="sm">{{pl.text}}</Tag>
                      {{/let}}
                    </td>
                    <td>{{if m.agenda_count m.agenda_count 0}}</td>
                    <td>{{if m.documents m.documents 0}}</td>
                  </tr>
                {{/each}}
              </tbody>
            </table>
          </div>
          {{#if this.paged}}
            <Pagination
              @page={{c.page}}
              @pageSize={{MEETINGS_PAGE}}
              @total={{this.total}}
              @onChange={{c.setPage}}
            />
          {{/if}}
        </div>
      {{else}}
        <div class="dis-empty">{{if
            c.upcoming
            "Keine kommenden Sitzungen."
            "Keine Sitzungen."
          }}</div>
      {{/if}}
    {{/let}}
  </template>
}
