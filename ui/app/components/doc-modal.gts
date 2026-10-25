import Component from '@glimmer/component';
import type { TOC } from '@ember/component/template-only';
import { cached, tracked } from '@glimmer/tracking';
import { getOwner } from '@ember/owner';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import type RouterService from '@ember/routing/router-service';
import { modifier } from 'ember-modifier';
import CanonButton from 'delphicanon/components/canon-button';
import CanonGrid from 'delphicanon/components/canon-grid';
import CanonHeading from 'delphicanon/components/canon-heading';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import { fileUrl, getDocument, versionUrl, type Doc } from 'ui/api';
import type SucheController from 'ui/controllers/suche';
import DocsGlyph from 'ui/components/docs-glyph';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import Modal, { type PaneContext } from 'ui/components/modal';
import { SubmitterChips } from 'ui/components/party-chip';
import Tag from 'ui/components/tag';
import rough from 'ui/modifiers/rough';
import shortcut from 'ui/modifiers/shortcut';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { relDate } from 'ui/utils/api-dokumente';
import { RIS_BASE, or } from 'ui/utils/meta';
import './doc-modal.css';

const EntityRow: TOC<{
  Args: {
    label: string;
    icon: string;
    items?: string[];
    onPick: (q: string) => void;
  };
}> = <template>
  {{#if @items.length}}
    <CanonRow @title={{@label}} @icon={{component Icon name=@icon}}>
      <span class="dis-tags dis-doc-tags">
        {{#each @items as |i|}}
          <Tag @onClick={{fn @onPick i}}>{{i}}</Tag>
        {{/each}}
      </span>
    </CanonRow>
  {{/if}}
</template>;

const scrollTop = modifier((el: HTMLElement, [id]: [string]) => {
  void id;
  for (let n: HTMLElement | null = el; n; n = n.parentElement) {
    n.scrollTop = 0;
    if (n.tagName === 'DIALOG') break;
  }
});

const position = (i: number, n: number) => `${i + 1} / ${n}`;

const versionUntil = (s?: string) => (s || '?').slice(0, 16).replace('T', ' ');
const day = (s?: string) => (s || '?').slice(0, 10);
const kb = (n?: number) => (n ? ` · ${Math.round(n / 1024)} kB` : '');
const isHttp = (u?: string) => !!u?.startsWith('http');
const orDash = (s?: string) => s || '—';

interface Signature {
  Args: { id: string; onClose: () => void; pane?: PaneContext };
}

export default class DocModal extends Component<Signature> {
  @service declare nav: NavService;
  @service declare router: RouterService;
  @tracked copied = '';

  @cached
  get load() {
    return this.args.id
      ? new Load<Doc & { local_path?: string }>(getDocument(this.args.id))
      : undefined;
  }

  get doc() {
    return this.load?.value;
  }

  get list() {
    if (this.router.currentRouteName !== 'suche') return undefined;
    const c = getOwner(this)?.lookup('controller:suche') as
      SucheController | undefined;
    const docs = c?.results ?? [];
    const i = docs.findIndex((d) => d.id === this.args.id);
    return c && i >= 0 ? { c, docs, i } : undefined;
  }

  get prevDoc() {
    const l = this.list;
    return l ? l.docs[l.i - 1] : undefined;
  }

  get nextDoc() {
    const l = this.list;
    return l ? l.docs[l.i + 1] : undefined;
  }

  step = (d?: Doc) => {
    const l = this.list;
    if (!d || !l) return;
    l.c.setCursor(l.docs.indexOf(d));
    this.nav.replaceTop('doc', d.id);
  };

  prev = () => this.step(this.prevDoc);
  next = () => this.step(this.nextDoc);

  openPdf = () => window.open(this.pdfUrl, '_blank', 'noopener');

  copyLink = () => {
    navigator.clipboard.writeText(window.location.href).then(
      () => (this.copied = 'Kopiert'),
      () => (this.copied = 'Fehlgeschlagen'),
    );
    setTimeout(() => (this.copied = ''), 1500);
  };

  get heading() {
    return this.doc?.label || 'Dokument';
  }

  get ents() {
    return this.doc?.entities ?? {};
  }

  get hasEntities() {
    const e = this.ents;
    return (
      (e.people?.length || 0) +
        (e.orgs?.length || 0) +
        (e.locations?.length || 0) >
      0
    );
  }

  get referenced() {
    const d = this.doc;
    return (d?.vorlagen ?? []).filter(
      (v) => !v.own && v.vorlage !== d?.vorlage,
    );
  }

  get referencedShown() {
    return this.referenced.slice(0, 16);
  }

  get referencedMore() {
    return this.referenced.length > 16 ? this.referenced.length - 16 : 0;
  }

  get meetingUrl() {
    return `${RIS_BASE}/${this.doc?.meeting_id}`;
  }

  get agendaUrl() {
    const d = this.doc;
    return d?.meeting_id && d.agenda_anchor
      ? `${RIS_BASE}/${d.meeting_id}#${d.agenda_anchor}`
      : '';
  }

  get agendaText() {
    const d = this.doc;
    return `${d?.agenda_number ? `TOP ${d.agenda_number}` : 'TOP'}${d?.agenda_title ? ` — ${d.agenda_title}` : ''}`;
  }

  get agendaPlain() {
    const d = this.doc;
    return `${d?.agenda_number ? 'TOP ' + d.agenda_number : ''}${d?.agenda_title ? ' — ' + d.agenda_title : ''}`;
  }

  get pdfUrl() {
    return fileUrl(this.args.id);
  }

  versionHref = (vid: number) => versionUrl(this.args.id, vid);

  filter = (key: 'topic' | 'district' | 'street' | 'cat', value: string) =>
    this.nav.filter({ [key]: value });

  vorlage = (nr: string) => this.nav.open({ vorlage: nr });

  searchFor = (q: string) => this.nav.filter({ q });

  <template>
    <Modal @heading={{this.heading}} @pane={{@pane}} @onClose={{@onClose}}>
      {{#if @id}}
        <div
          class="dis-doc-bar"
          {{scrollTop @id}}
          {{shortcut "ArrowLeft" this.prev}}
          {{shortcut "ArrowRight" this.next}}
          {{shortcut "k" this.prev}}
          {{shortcut "j" this.next}}
          {{shortcut "o" this.openPdf}}
        >
          {{#if this.list}}
            <CanonButton
              @kind="cell"
              @icon={{component Icon name="chevron-left"}}
              @label="Voriges Dokument"
              @disabled={{if this.prevDoc false true}}
              @onClick={{this.prev}}
            />
            <span class="dis-doc-pos">{{position
                this.list.i
                this.list.docs.length
              }}</span>
            <CanonButton
              @kind="cell"
              @icon={{component Icon name="chevron-right"}}
              @label="Nächstes Dokument"
              @disabled={{if this.nextDoc false true}}
              @onClick={{this.next}}
            />
          {{/if}}
          <span class="dis-doc-bar-end">
            <CanonButton
              @kind="cell"
              @text={{if this.copied this.copied "Link kopieren"}}
              @icon={{component Icon name="docs-link"}}
              @onClick={{this.copyLink}}
            />
            <a
              class="dis-doc-action dis-doc-action--primary"
              href={{this.pdfUrl}}
              target="_blank"
              rel="noopener noreferrer"
              aria-keyshortcuts="o"
            >
              PDF öffnen
              <Icon @name="launch" />
            </a>
          </span>
        </div>
      {{/if}}
      <LoadState @load={{this.load}}>
        {{#let this.doc as |doc|}}
          {{#if doc}}
            <CanonRows class="dis-doc">
              <div class="dis-doc-lead">
                <span class="dis-doc-kicker">
                  <DocsGlyph @type={{doc.doc_type}} />
                  <span>{{doc.doc_type}}</span>
                  {{#if doc.body_name}}<span>· {{doc.body_name}}</span>{{/if}}
                  {{#if doc.meeting_date}}
                    <time
                      datetime={{doc.meeting_date}}
                      title={{doc.meeting_date}}
                    >· {{relDate doc.meeting_date}}</time>
                  {{/if}}
                </span>
                {{#if doc.agenda_title}}
                  <p class="dis-doc-subject">{{doc.agenda_title}}</p>
                {{/if}}
                {{#if doc.summary_de}}
                  <p
                    class="dis-doc-summary"
                    {{rough "bracket" brackets="left" padding=2 delay=300}}
                  >{{doc.summary_de}}</p>
                {{/if}}
                {{#if doc.topics.length}}
                  <div class="dis-tags" aria-label="Schlagworte" role="group">
                    <Icon class="dis-doc-topics-icon" @name="docs-topics" />
                    {{#each doc.topics as |t|}}
                      <Tag @onClick={{fn this.filter "topic" t}}>{{t}}</Tag>
                    {{/each}}
                  </div>
                {{/if}}
              </div>

              {{#if doc.local_path}}
                <CanonHeading @text="Vorschau" />
                <iframe
                  class="dis-doc-preview"
                  title="PDF-Vorschau"
                  src={{this.pdfUrl}}
                  loading="lazy"
                ></iframe>
              {{/if}}

              {{#if this.hasEntities}}
                <CanonHeading @text="Erwähnte Entitäten" />
                <EntityRow
                  @label="Personen"
                  @icon="docs-person"
                  @items={{this.ents.people}}
                  @onPick={{this.searchFor}}
                />
                <EntityRow
                  @label="Organisationen"
                  @icon="docs-org"
                  @items={{this.ents.orgs}}
                  @onPick={{this.searchFor}}
                />
                <EntityRow
                  @label="Orte"
                  @icon="docs-district"
                  @items={{this.ents.locations}}
                  @onPick={{this.searchFor}}
                />
              {{/if}}

              <CanonHeading @text="Angaben" />
              <CanonRow
                @title="Gremium"
                @icon={{component Icon name="docs-body"}}
              >
                <span class="dis-doc-value">{{orDash doc.body_name}}</span>
              </CanonRow>
              <CanonRow
                @title="Sitzung"
                @icon={{component Icon name="calendar"}}
              >
                <span class="dis-doc-value">
                  {{#if doc.meeting_id}}
                    <a
                      href={{this.meetingUrl}}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {{if doc.meeting_title doc.meeting_title doc.meeting_id}}
                      <Icon @name="launch" @size={{12}} />
                    </a>
                  {{else}}
                    {{orDash doc.meeting_title}}
                  {{/if}}
                </span>
              </CanonRow>
              {{#if (or doc.agenda_number doc.agenda_title)}}
                <CanonRow
                  @title="Tagesordnungspunkt"
                  @icon={{component Icon name="docs-top"}}
                >
                  <span class="dis-doc-value">
                    {{#if this.agendaUrl}}
                      <a
                        href={{this.agendaUrl}}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        {{this.agendaText}}
                        <Icon @name="launch" @size={{12}} />
                      </a>
                    {{else}}
                      {{this.agendaPlain}}
                    {{/if}}
                  </span>
                </CanonRow>
              {{/if}}
              <CanonRow @title="Datum" @icon={{component Icon name="docs-day"}}>
                <span class="dis-doc-value">{{orDash doc.meeting_date}}</span>
              </CanonRow>
              <CanonRow @title="Typ" @icon={{component Icon name="docs-type"}}>
                <span class="dis-doc-value">{{doc.doc_type}}</span>
              </CanonRow>
              {{#if doc.submitters.length}}
                <CanonRow
                  @title="Einbringer"
                  @icon={{component Icon name="users"}}
                >
                  <SubmitterChips @codes={{doc.submitters}} />
                </CanonRow>
              {{/if}}
              {{#if doc.category}}
                <CanonRow
                  @title="Kategorie"
                  @icon={{component Icon name="docs-category"}}
                >
                  <Tag
                    @onClick={{fn this.filter "cat" doc.category}}
                  >{{doc.category}}</Tag>
                </CanonRow>
              {{/if}}
              {{#if doc.district}}
                <CanonRow
                  @title="Stadtteil"
                  @icon={{component Icon name="docs-district"}}
                >
                  <Tag
                    @onClick={{fn this.filter "district" doc.district}}
                    title="Dokumente aus diesem Stadtteil anzeigen"
                  >{{doc.district}}</Tag>
                </CanonRow>
              {{/if}}
              {{#if doc.vorlage}}
                <CanonRow
                  @title="Vorlage"
                  @icon={{component Icon name="docs-route"}}
                >
                  <Tag
                    @onClick={{fn this.vorlage doc.vorlage}}
                    title="Werdegang dieser Vorlage anzeigen"
                  >{{doc.vorlage}}</Tag>
                </CanonRow>
              {{/if}}

              {{#if doc.summary_en}}
                <CanonHeading @text="Summary (EN)" />
                <p class="dis-doc-text" lang="en">{{doc.summary_en}}</p>
              {{/if}}

              {{#if doc.streets.length}}
                <CanonHeading @text="Straßen" />
                <div class="dis-tags dis-doc-block">
                  {{#each doc.streets as |s|}}
                    <Tag @onClick={{fn this.filter "street" s}}>{{s}}</Tag>
                  {{/each}}
                </div>
              {{/if}}

              {{#if this.referenced.length}}
                <CanonHeading @text="Erwähnte Vorlagen" />
                <div class="dis-tags dis-doc-block">
                  {{#each this.referencedShown as |v|}}
                    <Tag
                      @onClick={{fn this.vorlage v.vorlage}}
                    >{{v.vorlage}}</Tag>
                  {{/each}}
                  {{#if this.referencedMore}}
                    <span class="dis-doc-muted">+{{this.referencedMore}}
                      weitere</span>
                  {{/if}}
                </div>
              {{/if}}

              {{#if doc.versions.length}}
                <CanonHeading
                  @text="Frühere Versionen ({{doc.versions.length}})"
                />
                <ul class="dis-doc-block dis-doc-versions">
                  {{#each doc.versions key="id" as |v|}}
                    <li>
                      <a
                        href={{this.versionHref v.id}}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <Icon @name="docs-history" @size={{14}} />
                        Version bis
                        {{versionUntil v.superseded_at}}
                      </a>
                      <span class="dis-doc-muted">
                        · abgerufen
                        {{day v.downloaded_at}}{{kb v.size}}
                      </span>
                    </li>
                  {{/each}}
                </ul>
                <p class="dis-doc-text dis-doc-muted dis-doc-changed">
                  Dieses Dokument wurde im RIS
                  <span
                    {{rough
                      "underline"
                      color="--dis-highlight"
                      padding=1
                      multiline=true
                      delay=500
                    }}
                  >nachträglich verändert</span>; ältere Fassungen bleiben hier
                  archiviert.
                </p>
              {{/if}}

              {{#if (isHttp doc.url)}}
                <CanonGrid>
                  <a
                    class="dis-doc-action"
                    href={{doc.url}}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Original im RIS
                    <Icon @name="launch" />
                  </a>
                </CanonGrid>
              {{/if}}
              <p class="dis-doc-text dis-doc-muted">
                Datei-ID
                {{doc.id}}
                · Text:
                {{orDash doc.text_status}}
                · Analyse:
                {{orDash doc.enrich_status}}
              </p>
            </CanonRows>
          {{/if}}
        {{/let}}
      </LoadState>
    </Modal>
  </template>
}
