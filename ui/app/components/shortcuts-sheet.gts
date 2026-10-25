import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import type RouterService from '@ember/routing/router-service';
import { modifier } from 'ember-modifier';
import CanonSheet from 'delphicanon/components/canon-sheet';
import { VIEWS } from 'ui/components/palette-dialog';
import shortcut, { topDialog, typing } from 'ui/modifiers/shortcut';
import './shortcuts-sheet.css';

export const MOD = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Strg';

// each entry: alternatives, each a key sequence; edit freely, the sheet renders this list
export const SHORTCUTS: {
  group: string;
  items: { keys: string[][]; label: string }[];
}[] = [
  {
    group: 'Überall',
    items: [
      { keys: [[MOD, 'K']], label: 'Befehle' },
      ...VIEWS.map((v) => ({
        keys: [['G', v.key.toUpperCase()]],
        label: v.label,
      })),
      { keys: [['?']], label: 'Tastenkürzel' },
      { keys: [['Esc']], label: 'Ebene schließen' },
    ],
  },
  {
    group: 'Dokumente',
    items: [
      { keys: [['/']], label: 'Suche' },
      { keys: [['Esc']], label: 'Suche leeren' },
      { keys: [['J'], ['K']], label: 'Treffer wechseln' },
      { keys: [['↵']], label: 'Öffnen' },
    ],
  },
  {
    group: 'Dokument',
    items: [
      { keys: [['←'], ['→'], ['J'], ['K']], label: 'Voriges / nächstes' },
      { keys: [['O']], label: 'PDF öffnen' },
    ],
  },
  {
    group: 'Sitzungen',
    items: [
      { keys: [['←'], ['↑'], ['→'], ['↓']], label: 'Tag wechseln' },
      { keys: [['Pos1'], ['Ende']], label: 'Wochenanfang / -ende' },
      { keys: [['↵']], label: 'Tag öffnen' },
      {
        keys: [['Bild ↑'], ['Bild ↓'], ['['], [']']],
        label: 'Zeitraum wechseln',
      },
      { keys: [['T']], label: 'Heute' },
      { keys: [['M']], label: 'Monat' },
      { keys: [['W']], label: 'Woche' },
      { keys: [['L']], label: 'Liste' },
    ],
  },
  {
    group: 'Sitzung',
    items: [
      { keys: [['J'], ['K']], label: 'TOP wechseln' },
      { keys: [['↵'], ['Leer']], label: 'TOP auf/zu' },
      { keys: [['E']], label: 'Alle auf/zu' },
      { keys: [['F']], label: 'Filter wechseln' },
      {
        keys: [['H'], ['L'], ['['], [']']],
        label: 'Vorige / nächste Sitzung',
      },
    ],
  },
  {
    group: 'Person',
    items: [{ keys: [['['], [']']], label: 'Voriges / nächstes Mitglied' }],
  },
  {
    group: 'Gremien',
    items: [
      { keys: [['/']], label: 'Gremien filtern' },
      { keys: [['J'], ['K']], label: 'Gremium wechseln' },
      { keys: [['↵']], label: 'Öffnen' },
    ],
  },
  {
    group: 'Reden',
    items: [{ keys: [['['], [']']], label: 'Vorige / nächste Sitzung' }],
  },
];

const GO = Object.fromEntries(VIEWS.map((v) => [v.key, v.route]));
const GO_TIMEOUT = 1500;
const isLast = (list: unknown[], i: number) => i === list.length - 1;

// Owns the global keys: ? opens this sheet, g then a view letter switches view.
export default class ShortcutsSheet extends Component {
  @service declare router: RouterService;

  @tracked open = false;
  @tracked armed = false;
  #timer?: ReturnType<typeof setTimeout>;

  show = () => (this.open = true);
  hide = () => (this.open = false);

  disarm = () => {
    clearTimeout(this.#timer);
    this.armed = false;
  };

  // capture phase, so the second key never reaches a view's own binding
  sequence = modifier(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return;
      if (typing(e.target) || topDialog()) return this.disarm();
      const key = e.key.toLowerCase();
      if (this.armed) {
        this.disarm();
        const route = GO[key];
        if (!route) return;
        e.preventDefault();
        e.stopImmediatePropagation();
        this.router.transitionTo(route);
      } else if (key === 'g' && !e.shiftKey) {
        e.preventDefault();
        this.armed = true;
        this.#timer = setTimeout(this.disarm, GO_TIMEOUT);
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => {
      window.removeEventListener('keydown', onKey, true);
      clearTimeout(this.#timer);
    };
  });

  <template>
    <span
      class="dis-go {{if this.armed 'dis-go--armed'}}"
      aria-hidden="true"
      {{this.sequence}}
      {{shortcut "?" this.show}}
    ><kbd>G</kbd> …</span>
    <CanonSheet
      class="dis-keys-sheet"
      @open={{this.open}}
      @edge="end"
      @title="Tastenkürzel"
      @onClose={{this.hide}}
    >
      <div class="dis-keys">
        {{#each SHORTCUTS as |g|}}
          <section class="dis-keys-group">
            <h3 class="dis-keys-heading">{{g.group}}</h3>
            <dl class="dis-keys-list">
              {{#each g.items as |s|}}
                <div class="dis-keys-row">
                  <dt>
                    {{#each s.keys as |seq i|}}
                      <span class="dis-keys-seq">
                        {{#each seq as |k|}}<kbd>{{k}}</kbd>{{/each}}
                      </span>
                      {{#unless (isLast s.keys i)}}<span
                          class="dis-keys-or"
                        >/</span>{{/unless}}
                    {{/each}}
                  </dt>
                  <dd>{{s.label}}</dd>
                </div>
              {{/each}}
            </dl>
          </section>
        {{/each}}
      </div>
    </CanonSheet>
  </template>
}
