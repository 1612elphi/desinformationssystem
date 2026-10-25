import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { hash } from '@ember/helper';
import { on } from '@ember/modifier';
import { LinkTo } from '@ember/routing';
import type RouterService from '@ember/routing/router-service';
import Icon from 'ui/components/icon';
import shortcut from 'ui/modifiers/shortcut';
import type NavService from 'ui/services/nav';
import { COUNCIL } from 'ui/utils/api-uebersicht';
import './dash-quick.css';

const ICS = `/api/meetings.ics?committee=${encodeURIComponent(COUNCIL)}`;

export class DashSearch extends Component {
  @service declare router: RouterService;
  @tracked q = '';

  input = (e: Event) => (this.q = (e.target as HTMLInputElement).value);

  submit = (e: SubmitEvent) => {
    e.preventDefault();
    const q = this.q.trim();
    this.router.transitionTo('suche', { queryParams: q ? { q } : {} });
  };

  focus = () => document.getElementById('dis-dash-q')?.focus();

  <template>
    <form
      class="dis-dsearch"
      role="search"
      {{on "submit" this.submit}}
      {{shortcut "/" this.focus}}
    >
      <label class="dis-dsearch-label" for="dis-dash-q">
        <Icon @name="dash-search" @size={{22}} />
        <span class="dis-visually-hidden">Suche</span>
      </label>
      <input
        id="dis-dash-q"
        class="dis-dsearch-input"
        type="search"
        autocomplete="off"
        enterkeyhint="search"
        placeholder="Dokumente durchsuchen…"
        value={{this.q}}
        {{on "input" this.input}}
      />
      <kbd class="dis-dsearch-key">/</kbd>
    </form>
  </template>
}

export default class DashQuick extends Component<{
  Args: { lastId?: string };
}> {
  @service declare nav: NavService;

  openLast = () => {
    if (this.args.lastId) this.nav.open({ meeting: this.args.lastId });
  };

  <template>
    <nav class="dis-dquick" aria-labelledby="dis-dquick-title">
      <h2 id="dis-dquick-title" class="dis-dash-heading">
        <Icon @name="dash-go" @size={{18}} />
        Schnellzugriff
      </h2>
      <ul class="dis-dquick-tiles">
        <li>
          <LinkTo
            class="dis-dquick-tile"
            @route="sitzungen"
            @query={{hash view="monat"}}
          >
            <Icon @name="dash-calendar" @size={{22}} />
            <span>Sitzungen</span>
            <small>Kalender</small>
          </LinkTo>
        </li>
        <li>
          <LinkTo class="dis-dquick-tile" @route="gremien">
            <Icon @name="dash-body" @size={{22}} />
            <span>Gremien</span>
            <small>{{COUNCIL}}</small>
          </LinkTo>
        </li>
        <li>
          <LinkTo class="dis-dquick-tile" @route="reden">
            <Icon @name="dash-speakers" @size={{22}} />
            <span>Reden</span>
          </LinkTo>
        </li>
        <li>
          <LinkTo class="dis-dquick-tile" @route="statistik">
            <Icon @name="dash-stats" @size={{22}} />
            <span>Statistik</span>
          </LinkTo>
        </li>
        <li>
          <LinkTo class="dis-dquick-tile" @route="karte">
            <Icon @name="dash-map" @size={{22}} />
            <span>Karte</span>
          </LinkTo>
        </li>
        <li>
          <a class="dis-dquick-tile" href={{ICS}}>
            <Icon @name="dash-ics" @size={{22}} />
            <span>Kalender-Abo</span>
            <small>ICS</small>
          </a>
        </li>
        <li>
          <button
            type="button"
            class="dis-dquick-tile"
            disabled={{unless @lastId true}}
            {{on "click" this.openLast}}
          >
            <Icon @name="dash-last" @size={{22}} />
            <span>Letzter Gemeinderat</span>
          </button>
        </li>
      </ul>
    </nav>
  </template>
}
