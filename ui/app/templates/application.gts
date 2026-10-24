import Component from '@glimmer/component';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import type RouterService from '@ember/routing/router-service';
import { pageTitle } from 'ember-page-title';
import CanonSidebar from 'delphicanon/components/canon-sidebar';
import CanonSidebarRow from 'delphicanon/components/canon-sidebar-row';
import CanonWindow from 'delphicanon/components/canon-window';
import type ApplicationController from 'ui/controllers/application';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import type { Overlay } from 'ui/services/nav';
import Icon from 'ui/components/icon';
import LiveBanner from 'ui/components/live-banner';
import DocModal from 'ui/components/doc-modal';
import MeetingModal from 'ui/components/meeting-modal';
import PersonModal from 'ui/components/person-modal';
import VorlageModal from 'ui/components/vorlage-modal';
import { RIS_START, scrapeTime } from 'ui/utils/meta';

const VIEWS = [
  { route: 'index', label: 'Dokumente', icon: 'document' },
  { route: 'sitzungen', label: 'Sitzungen', icon: 'calendar' },
  { route: 'gremien', label: 'Gremien', icon: 'users' },
  { route: 'statistik', label: 'Statistik', icon: 'chart' },
  { route: 'karte', label: 'Karte', icon: 'map' },
];

// The same path only works behind the path-routing tunnel (dis.delphi.tools);
// on a direct deployment the MCP server listens on its own port.
const MCP_HREF =
  window.location.hostname === 'dis.delphi.tools'
    ? '/mcp'
    : `${window.location.protocol}//${window.location.hostname}:3651/mcp`;

const bool = (v: unknown) => !!v;

interface Signature {
  Args: { model: ApplicationModel; controller: ApplicationController };
}

export default class ApplicationTemplate extends Component<Signature> {
  @service declare router: RouterService;
  @service declare nav: NavService;

  isView = (route: string) => this.router.currentRouteName === route;
  href = (route: string) => this.router.urlFor(route);

  go = (route: string, event: MouseEvent) => {
    event.preventDefault();
    this.router.transitionTo(route);
  };

  // the sheet reports every close; one the URL already made needs nothing
  close = (key: Overlay) => {
    if (this.args.controller[key]) this.nav.close(key);
  };

  get stats() {
    return this.args.model.stats.value;
  }

  <template>
    {{pageTitle "Desinformationssystem Karlsruhe"}}
    <CanonWindow class="dis-window" @title="Desinformationssystem">
      <:toolbar>
        <div class="dis-toolbar">
          <a href={{RIS_START}} target="_blank" rel="noopener noreferrer">
            Original-RIS
            <Icon @name="launch" @size={{14}} />
          </a>
        </div>
      </:toolbar>
      <:sidebar>
        <CanonSidebar aria-label="Ansichten">
          {{#each VIEWS as |v|}}
            <CanonSidebarRow
              @title={{v.label}}
              @icon={{component Icon name=v.icon size=20}}
              @href={{this.href v.route}}
              @selected={{this.isView v.route}}
              @onClick={{fn this.go v.route}}
            />
          {{/each}}
        </CanonSidebar>
      </:sidebar>
      <:main>
        <main class="dis-main">
          <LiveBanner />
          {{outlet}}
          <footer class="dis-footer">
            <span>
              Quelle:
              <a
                href={{RIS_START}}
                target="_blank"
                rel="noopener noreferrer"
              >Sitzungskalender der Stadt Karlsruhe</a>
            </span>
            <span>Open Data (CC BY-ND)</span>
            <span>MCP: <a href={{MCP_HREF}}>/mcp</a></span>
            {{#if this.stats.last_scrape}}
              <span>Stand {{scrapeTime this.stats.last_scrape}}</span>
            {{/if}}
          </footer>
        </main>
      </:main>
    </CanonWindow>

    <MeetingModal
      @id={{@controller.meeting}}
      @open={{bool @controller.meeting}}
      @onClose={{fn this.close "meeting"}}
    />
    <DocModal
      @id={{@controller.doc}}
      @open={{bool @controller.doc}}
      @onClose={{fn this.close "doc"}}
    />
    <PersonModal
      @id={{@controller.person}}
      @open={{bool @controller.person}}
      @onClose={{fn this.close "person"}}
    />
    <VorlageModal
      @nr={{@controller.vorlage}}
      @open={{bool @controller.vorlage}}
      @onClose={{fn this.close "vorlage"}}
    />
  </template>
}
