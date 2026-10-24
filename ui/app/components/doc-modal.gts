import Component from '@glimmer/component';
import type { TOC } from '@ember/component/template-only';
import { cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { fileUrl, getDocument, versionUrl } from 'ui/api';
import Icon from 'ui/components/icon';
import Modal from 'ui/components/modal';
import { SubmitterChips } from 'ui/components/party-chip';
import Tag from 'ui/components/tag';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { RIS_BASE, or } from 'ui/utils/meta';

const EntityRow: TOC<{ Args: { label: string; items?: string[] } }> = <template>
  {{#if @items.length}}
    <div class="dis-entity-row">
      <span class="dis-meta-line">{{@label}}: </span>
      <span class="dis-tags dis-tags--inline">
        {{#each @items as |i|}}
          <Tag @type="cool-gray" @size="sm">{{i}}</Tag>
        {{/each}}
      </span>
    </div>
  {{/if}}
</template>;

const versionUntil = (s?: string) => (s || '?').slice(0, 16).replace('T', ' ');
const day = (s?: string) => (s || '?').slice(0, 10);
const kb = (n?: number) => (n ? ` · ${Math.round(n / 1024)} kB` : '');
const isHttp = (u?: string) => !!u?.startsWith('http');
const orDash = (s?: string) => s || '—';

interface Signature {
  Args: { id: string; onClose: () => void };
}

export default class DocModal extends Component<Signature> {
  @service declare nav: NavService;

  @cached
  get load() {
    return new Load(getDocument(this.args.id));
  }

  get doc() {
    return this.load.value;
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

  filter = (key: 'topic' | 'district' | 'street', value: string) =>
    this.nav.filter({ [key]: value });

  vorlage = (nr: string) => this.nav.open({ doc: '', vorlage: nr });

  <template>
    {{#let this.doc as |doc|}}
      {{#if doc}}
        <Modal @heading={{doc.label}} @onClose={{@onClose}}>
          <div class="dis-detail">
            <dl>
              <dt>Gremium</dt>
              <dd>{{orDash doc.body_name}}</dd>
              <dt>Sitzung</dt>
              <dd>
                {{#if doc.meeting_id}}
                  <a
                    class="cds--link"
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
              </dd>
              {{#if (or doc.agenda_number doc.agenda_title)}}
                <dt>Tagesordnungspunkt</dt>
                <dd>
                  {{#if this.agendaUrl}}
                    <a
                      class="cds--link"
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
                </dd>
              {{/if}}
              <dt>Datum</dt>
              <dd>{{orDash doc.meeting_date}}</dd>
              <dt>Typ</dt>
              <dd>{{doc.doc_type}}</dd>
              {{#if doc.submitters.length}}
                <dt>Einbringer</dt>
                <dd><SubmitterChips @codes={{doc.submitters}} /></dd>
              {{/if}}
              {{#if doc.category}}
                <dt>Kategorie</dt>
                <dd><Tag @type="blue" @size="sm">{{doc.category}}</Tag></dd>
              {{/if}}
              {{#if doc.district}}
                <dt>Stadtteil</dt>
                <dd>
                  <Tag
                    @type="teal"
                    @size="sm"
                    @onClick={{fn this.filter "district" doc.district}}
                    title="Dokumente aus diesem Stadtteil anzeigen"
                  >{{doc.district}}</Tag>
                </dd>
              {{/if}}
              {{#if doc.vorlage}}
                <dt>Vorlage</dt>
                <dd>
                  <Tag
                    @type="purple"
                    @size="sm"
                    @onClick={{fn this.vorlage doc.vorlage}}
                    title="Werdegang dieser Vorlage anzeigen"
                  >{{doc.vorlage}}</Tag>
                </dd>
              {{/if}}
            </dl>

            {{#if doc.summary_de}}
              <h4>Zusammenfassung</h4>
              <p class="summary">{{doc.summary_de}}</p>
            {{/if}}
            {{#if doc.summary_en}}
              <h4>Summary (EN)</h4>
              <p class="summary">{{doc.summary_en}}</p>
            {{/if}}

            {{#if doc.topics.length}}
              <h4>Schlagworte</h4>
              <div class="dis-tags">
                {{#each doc.topics as |t|}}
                  <Tag
                    @type="teal"
                    @onClick={{fn this.filter "topic" t}}
                  >{{t}}</Tag>
                {{/each}}
              </div>
            {{/if}}

            {{#if doc.streets.length}}
              <h4>Straßen</h4>
              <div class="dis-tags">
                {{#each doc.streets as |s|}}
                  <Tag
                    @type="green"
                    @onClick={{fn this.filter "street" s}}
                  >{{s}}</Tag>
                {{/each}}
              </div>
            {{/if}}

            {{#if this.hasEntities}}
              <h4>Erwähnte Entitäten</h4>
              <EntityRow @label="Personen" @items={{this.ents.people}} />
              <EntityRow @label="Organisationen" @items={{this.ents.orgs}} />
              <EntityRow @label="Orte" @items={{this.ents.locations}} />
            {{/if}}

            {{#if this.referenced.length}}
              <h4>Erwähnte Vorlagen</h4>
              <div class="dis-tags">
                {{#each this.referencedShown as |v|}}
                  <Tag
                    @type="purple"
                    @size="sm"
                    @onClick={{fn this.vorlage v.vorlage}}
                  >{{v.vorlage}}</Tag>
                {{/each}}
                {{#if this.referencedMore}}
                  <span class="dis-meta-line">+{{this.referencedMore}}
                    weitere</span>
                {{/if}}
              </div>
            {{/if}}

            {{#if doc.versions.length}}
              <h4>Frühere Versionen ({{doc.versions.length}})</h4>
              <ul class="dis-versions">
                {{#each doc.versions key="id" as |v|}}
                  <li>
                    <a
                      href={{this.versionHref v.id}}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      Version bis
                      {{versionUntil v.superseded_at}}
                    </a>
                    <span class="dis-meta-line">
                      · abgerufen
                      {{day v.downloaded_at}}{{kb v.size}}
                    </span>
                  </li>
                {{/each}}
              </ul>
              <p class="dis-meta-line">
                Dieses Dokument wurde im RIS nachträglich verändert; ältere
                Fassungen bleiben hier archiviert.
              </p>
            {{/if}}

            <div class="dis-doc-actions">
              <a
                class="cds--btn cds--btn--primary"
                href={{this.pdfUrl}}
                target="_blank"
                rel="noopener noreferrer"
              >
                PDF öffnen
                <Icon @name="document" class="cds--btn__icon" />
              </a>
              {{#if (isHttp doc.url)}}
                <a
                  class="cds--btn cds--btn--tertiary"
                  href={{doc.url}}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Original im RIS
                  <Icon @name="launch" class="cds--btn__icon" />
                </a>
              {{/if}}
            </div>
            <p class="dis-meta-line dis-spaced">
              Datei-ID
              {{doc.id}}
              · Text:
              {{orDash doc.text_status}}
              · Analyse:
              {{orDash doc.enrich_status}}
            </p>
          </div>
        </Modal>
      {{/if}}
    {{/let}}
  </template>
}
