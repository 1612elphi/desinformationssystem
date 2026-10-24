import Component from '@glimmer/component';
import { service } from '@ember/service';
import { LinkTo } from '@ember/routing';
import { fn } from '@ember/helper';
import type RouterService from '@ember/routing/router-service';
import { pageTitle } from 'ember-page-title';
import type ApplicationController from 'ui/controllers/application';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import Icon from 'ui/components/icon';
import Tag from 'ui/components/tag';
import LiveBanner from 'ui/components/live-banner';
import DocModal from 'ui/components/doc-modal';
import MeetingModal from 'ui/components/meeting-modal';
import PersonModal from 'ui/components/person-modal';
import VorlageModal from 'ui/components/vorlage-modal';
import { RIS_START, num, scrapeTime } from 'ui/utils/meta';

const TABS = [
  { route: 'index', label: 'Dokumente' },
  { route: 'sitzungen', label: 'Sitzungen' },
  { route: 'gremien', label: 'Gremien' },
  { route: 'statistik', label: 'Statistik' },
  { route: 'karte', label: 'Karte' },
];

// The same path only works behind the path-routing tunnel (dis.delphi.tools);
// on a direct deployment the MCP server listens on its own port.
const MCP_HREF =
  window.location.hostname === 'dis.delphi.tools'
    ? '/mcp'
    : `${window.location.protocol}//${window.location.hostname}:3651/mcp`;

interface Signature {
  Args: { model: ApplicationModel; controller: ApplicationController };
}

export default class ApplicationTemplate extends Component<Signature> {
  @service declare router: RouterService;
  @service declare nav: NavService;

  isTab = (route: string) => this.router.currentRouteName === route;

  close = (key: 'doc' | 'meeting' | 'person' | 'vorlage') =>
    this.nav.open({ [key]: '' });

  get stats() {
    return this.args.model.stats.value;
  }

  <template>
    {{pageTitle "Desinformationssystem — Karlsruhe"}}
    <header class="cds--header" aria-label="Desinformationssystem">
      <a class="cds--header__name" href="/">
        <span
          class="cds--header__name--prefix"
        >Karlsruhe</span>&nbsp;Desinformationssystem
      </a>
      <div class="cds--header__global">
        <div class="dis-header-right">
          <Tag @type="cool-gray" @size="sm">Open Data</Tag>
          <a
            class="dis-header-link"
            href={{RIS_START}}
            target="_blank"
            rel="noopener noreferrer"
          >
            Original-RIS
            <Icon @name="launch" @size={{14}} />
          </a>
        </div>
      </div>
    </header>

    <main class="dis-content">
      <div class="dis-hero">
        <h1>Karlsruher Ratsdokumente</h1>
        <p class="dis-sub">
          Durchsuchbares Archiv des Sitzungskalenders der Stadt Karlsruhe —
          täglich aktualisiert, volltextindiziert und kategorisiert.
        </p>
      </div>

      {{#if this.stats}}
        <div class="dis-stats">
          <div class="dis-stat"><div class="num">{{num
                this.stats.documents
              }}</div><div class="lbl">Dokumente</div></div>
          <div class="dis-stat"><div class="num">{{num
                this.stats.meetings
              }}</div><div class="lbl">Sitzungen</div></div>
          <div class="dis-stat"><div class="num">{{num
                this.stats.bodies
              }}</div><div class="lbl">Gremien</div></div>
          <div class="dis-stat"><div class="num">{{num
                this.stats.with_text
              }}</div><div class="lbl">Volltext</div></div>
          <div class="dis-stat"><div class="num">{{num
                this.stats.enriched
              }}</div><div class="lbl">Analysiert</div></div>
          <div class="dis-stat">
            <div class="num dis-stat-time">{{scrapeTime
                this.stats.last_scrape
              }}</div>
            <div class="lbl">Letzter Abruf</div>
          </div>
        </div>
      {{/if}}

      <LiveBanner />

      <div class="cds--tabs cds--tabs--contained">
        <div class="cds--tab--list" role="tablist" aria-label="Ansichten">
          {{#each TABS as |t|}}
            <LinkTo
              @route={{t.route}}
              role="tab"
              aria-selected={{if (this.isTab t.route) "true" "false"}}
              class="cds--tabs__nav-item cds--tabs__nav-link
                {{if (this.isTab t.route) 'cds--tabs__nav-item--selected'}}"
            >
              <div class="cds--tabs__nav-item-label-wrapper">
                <span class="cds--tabs__nav-item-label">{{t.label}}</span>
              </div>
            </LinkTo>
          {{/each}}
        </div>
      </div>
      <div class="cds--tab-content" role="tabpanel">
        {{outlet}}
      </div>
    </main>

    <footer class="dis-footer">
      <span>Desinformationssystem</span>
      <span class="sep">·</span>
      <span>
        Quelle:
        <a
          href={{RIS_START}}
          target="_blank"
          rel="noopener noreferrer"
        >Sitzungskalender der Stadt Karlsruhe</a>
      </span>
      <span class="sep">·</span>
      <span>Open Data (CC BY-ND)</span>
      <span class="sep">·</span>
      <span>MCP: <a href={{MCP_HREF}}>/mcp</a></span>
      {{#if this.stats.last_scrape}}
        <span class="sep">·</span>
        <span>Stand {{scrapeTime this.stats.last_scrape}}</span>
      {{/if}}
    </footer>

    {{#if @controller.meeting}}
      <MeetingModal
        @id={{@controller.meeting}}
        @onClose={{fn this.close "meeting"}}
      />
    {{/if}}
    {{#if @controller.doc}}
      <DocModal @id={{@controller.doc}} @onClose={{fn this.close "doc"}} />
    {{/if}}
    {{#if @controller.person}}
      <PersonModal
        @id={{@controller.person}}
        @onClose={{fn this.close "person"}}
      />
    {{/if}}
    {{#if @controller.vorlage}}
      <VorlageModal
        @nr={{@controller.vorlage}}
        @onClose={{fn this.close "vorlage"}}
      />
    {{/if}}
  </template>
}
