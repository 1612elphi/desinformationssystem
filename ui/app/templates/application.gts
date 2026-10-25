import Component from '@glimmer/component';
import { service } from '@ember/service';
import { tracked } from '@glimmer/tracking';
import { concat, fn } from '@ember/helper';
import type RouterService from '@ember/routing/router-service';
import { pageTitle } from 'ember-page-title';
import CanonButton from 'delphicanon/components/canon-button';
import CanonSidebar from 'delphicanon/components/canon-sidebar';
import CanonSidebarRow from 'delphicanon/components/canon-sidebar-row';
import CanonWindow from 'delphicanon/components/canon-window';
import type ApplicationController from 'ui/controllers/application';
import type { ApplicationModel } from 'ui/routes/application';
import type NavService from 'ui/services/nav';
import { parseStack } from 'ui/services/nav';
import Icon from 'ui/components/icon';
import LiveBanner from 'ui/components/live-banner';
import PaneStack from 'ui/components/pane-stack';
import PaletteDialog, { VIEWS } from 'ui/components/palette-dialog';
import ShortcutsSheet, { MOD } from 'ui/components/shortcuts-sheet';
import shortcut from 'ui/modifiers/shortcut';
import { getLastCouncil } from 'ui/utils/api-reden';
import Load from 'ui/utils/load';
import { RIS_START, scrapeTime } from 'ui/utils/meta';
import './application.css';

const PHOTO_CREDIT = 'Porträts: Stadt Karlsruhe, Müller-Gmelin, Fränkle';

const NO_NAZIS =
  'Nazis sind hier unerwünscht und damit meinen wir explizit die AfD ihr Faschos jaja';

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

  @tracked palette = false;

  lastCouncil = new Load(getLastCouncil());

  openLastCouncil = () => {
    const id = this.lastCouncil.value?.id;
    if (id) this.nav.open({ meeting: id });
  };

  get lastCouncilDate() {
    const d = this.lastCouncil.value?.date;
    return d ? d.split('-').reverse().join('.') : '';
  }

  openPalette = () => (this.palette = true);
  closePalette = () => (this.palette = false);

  isView = (route: string) => this.router.currentRouteName === route;
  href = (route: string) => this.router.urlFor(route);

  go = (route: string, event: MouseEvent) => {
    event.preventDefault();
    this.router.transitionTo(route);
  };

  get panes() {
    return parseStack(this.args.controller.stack);
  }

  popTo = (index: number) => this.nav.popTo(index);

  get stats() {
    return this.args.model.stats.value;
  }

  <template>
    {{pageTitle "Desinformationssystem Karlsruhe"}}
    <CanonWindow class="dis-window" @title="Desinformationssystem">
      <:toolbar>
        <div class="dis-toolbar">
          <CanonButton
            class="dis-toolbar-search"
            @kind="cell"
            @text="Suchen"
            @icon={{component Icon name="search"}}
            @onClick={{this.openPalette}}
            aria-keyshortcuts="Meta+K Control+K"
            data-keys={{concat MOD " K"}}
            {{shortcut "mod+k" this.openPalette}}
          />
          {{#if this.lastCouncil.value}}
            <CanonButton
              class="dis-toolbar-council"
              @kind="cell"
              @text="Letzter Gemeinderat"
              @icon={{component Icon name="calendar"}}
              @onClick={{this.openLastCouncil}}
              title={{this.lastCouncilDate}}
              data-date={{this.lastCouncilDate}}
            />
          {{/if}}
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
              data-keys={{concat "G " v.key}}
            />
          {{/each}}
          <aside class="dis-sidebar-card" title={{NO_NAZIS}}>
            <Icon @name="shield-x" @size={{20}} />
            <p>{{NO_NAZIS}}</p>
          </aside>
        </CanonSidebar>
      </:sidebar>
      <:main>
        <main class="dis-main">
          <LiveBanner />
          {{outlet}}
          <footer class="dis-footer">
            <span>
              <Icon @name="database" @size={{14}} />
              Quelle:
              <a
                href={{RIS_START}}
                target="_blank"
                rel="noopener noreferrer"
              >Sitzungskalender der Stadt Karlsruhe</a>
            </span>
            <span><Icon @name="scale" @size={{14}} />
              Open Data (CC BY-ND)</span>
            <span><Icon @name="camera" @size={{14}} /> {{PHOTO_CREDIT}}</span>
            <span><Icon @name="plug" @size={{14}} />
              MCP:
              <a href={{MCP_HREF}}>/mcp</a></span>
            {{#if this.stats.last_scrape}}
              <span><Icon @name="time" @size={{14}} />
                Stand
                {{scrapeTime this.stats.last_scrape}}</span>
            {{/if}}
          </footer>
        </main>
      </:main>
    </CanonWindow>

    <PaletteDialog @open={{this.palette}} @onClose={{this.closePalette}} />
    <ShortcutsSheet />
    <PaneStack @stack={{this.panes}} @onPopTo={{this.popTo}} />
  </template>
}
