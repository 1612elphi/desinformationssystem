import Component from '@glimmer/component';
import type { TOC } from '@ember/component/template-only';
import { cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import CanonGrid from 'delphicanon/components/canon-grid';
import CanonHeading from 'delphicanon/components/canon-heading';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import { fileUrl, getDocument, versionUrl } from 'ui/api';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import Modal from 'ui/components/modal';
import { SubmitterChips } from 'ui/components/party-chip';
import Tag from 'ui/components/tag';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { RIS_BASE, or } from 'ui/utils/meta';
import './doc-modal.css';

const EntityRow: TOC<{ Args: { label: string; items?: string[] } }> = <template>
  {{#if @items.length}}
    <CanonRow @title={{@label}}>
      <span class="dis-tags dis-doc-tags">
        {{#each @items as |i|}}
          <Tag>{{i}}</Tag>
        {{/each}}
      </span>
    </CanonRow>
  {{/if}}
</template>;

const versionUntil = (s?: string) => (s || '?').slice(0, 16).replace('T', ' ');
const day = (s?: string) => (s || '?').slice(0, 10);
const kb = (n?: number) => (n ? ` · ${Math.round(n / 1024)} kB` : '');
const isHttp = (u?: string) => !!u?.startsWith('http');
const orDash = (s?: string) => s || '—';

interface Signature {
  Args: { id: string; open: boolean; onClose: () => void };
}

export default class DocModal extends Component<Signature> {
  @service declare nav: NavService;

  @cached
  get load() {
    return this.args.id ? new Load(getDocument(this.args.id)) : undefined;
  }

  get doc() {
    return this.load?.value;
  }

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

  filter = (key: 'topic' | 'district' | 'street', value: string) =>
    this.nav.filter({ [key]: value });

  vorlage = (nr: string) => this.nav.open({ doc: '', vorlage: nr });

  <template>
    <Modal @heading={{this.heading}} @open={{@open}} @onClose={{@onClose}}>
      <LoadState @load={{this.load}}>
        {{#let this.doc as |doc|}}
          {{#if doc}}
            <CanonRows class="dis-doc">
              <CanonRow @title="Gremium">
                <span class="dis-doc-value">{{orDash doc.body_name}}</span>
              </CanonRow>
              <CanonRow @title="Sitzung">
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
                <CanonRow @title="Tagesordnungspunkt">
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
              <CanonRow @title="Datum">
                <span class="dis-doc-value">{{orDash doc.meeting_date}}</span>
              </CanonRow>
              <CanonRow @title="Typ">
                <span class="dis-doc-value">{{doc.doc_type}}</span>
              </CanonRow>
              {{#if doc.submitters.length}}
                <CanonRow @title="Einbringer">
                  <SubmitterChips @codes={{doc.submitters}} />
                </CanonRow>
              {{/if}}
              {{#if doc.category}}
                <CanonRow @title="Kategorie">
                  <Tag>{{doc.category}}</Tag>
                </CanonRow>
              {{/if}}
              {{#if doc.district}}
                <CanonRow @title="Stadtteil">
                  <Tag
                    @onClick={{fn this.filter "district" doc.district}}
                    title="Dokumente aus diesem Stadtteil anzeigen"
                  >{{doc.district}}</Tag>
                </CanonRow>
              {{/if}}
              {{#if doc.vorlage}}
                <CanonRow @title="Vorlage">
                  <Tag
                    @onClick={{fn this.vorlage doc.vorlage}}
                    title="Werdegang dieser Vorlage anzeigen"
                  >{{doc.vorlage}}</Tag>
                </CanonRow>
              {{/if}}

              {{#if doc.summary_de}}
                <CanonHeading @text="Zusammenfassung" />
                <p class="dis-doc-text">{{doc.summary_de}}</p>
              {{/if}}
              {{#if doc.summary_en}}
                <CanonHeading @text="Summary (EN)" />
                <p class="dis-doc-text" lang="en">{{doc.summary_en}}</p>
              {{/if}}

              {{#if doc.topics.length}}
                <CanonHeading @text="Schlagworte" />
                <div class="dis-tags dis-doc-block">
                  {{#each doc.topics as |t|}}
                    <Tag @onClick={{fn this.filter "topic" t}}>{{t}}</Tag>
                  {{/each}}
                </div>
              {{/if}}

              {{#if doc.streets.length}}
                <CanonHeading @text="Straßen" />
                <div class="dis-tags dis-doc-block">
                  {{#each doc.streets as |s|}}
                    <Tag @onClick={{fn this.filter "street" s}}>{{s}}</Tag>
                  {{/each}}
                </div>
              {{/if}}

              {{#if this.hasEntities}}
                <CanonHeading @text="Erwähnte Entitäten" />
                <EntityRow @label="Personen" @items={{this.ents.people}} />
                <EntityRow @label="Organisationen" @items={{this.ents.orgs}} />
                <EntityRow @label="Orte" @items={{this.ents.locations}} />
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
                <p class="dis-doc-text dis-doc-muted">
                  Dieses Dokument wurde im RIS nachträglich verändert; ältere
                  Fassungen bleiben hier archiviert.
                </p>
              {{/if}}

              <CanonGrid>
                <a
                  class="dis-doc-action dis-doc-action--primary"
                  href={{this.pdfUrl}}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  PDF öffnen
                  <Icon @name="document" />
                </a>
                {{#if (isHttp doc.url)}}
                  <a
                    class="dis-doc-action"
                    href={{doc.url}}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Original im RIS
                    <Icon @name="launch" />
                  </a>
                {{/if}}
              </CanonGrid>
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
