import Component from '@glimmer/component';
import { tracked, cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import CanonButton from 'delphicanon/components/canon-button';
import CanonRows from 'delphicanon/components/canon-rows';
import 'delphicanon/components/canon-heading.css';
import {
  getMeeting,
  refreshMeeting,
  type Doc,
  type Meeting,
  type Vote,
} from 'ui/api';
import DocRow from 'ui/components/doc-row';
import Icon from 'ui/components/icon';
import { InlineLoading, LoadState } from 'ui/components/loading';
import Modal from 'ui/components/modal';
import Tag from 'ui/components/tag';
import VoteBlock from 'ui/components/vote-block';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { RIS_BASE, hasTally, num, publicLabel, tally } from 'ui/utils/meta';
import './meeting-modal.css';

const concatTop = (n: string) => `TOP ${n}`;

interface Signature {
  Args: { id: string; open: boolean; onClose: () => void };
}

// State that belongs to one meeting id and resets when another one opens.
interface Local {
  id: string;
  fresh?: Meeting;
  busy: boolean;
  msg: string;
}

export default class MeetingModal extends Component<Signature> {
  @service declare nav: NavService;
  @tracked state: Local = { id: '', busy: false, msg: '' };

  @cached
  get load() {
    return this.args.id ? new Load(getMeeting(this.args.id)) : undefined;
  }

  get local(): Local {
    return this.state.id === this.args.id
      ? this.state
      : { id: this.args.id, busy: false, msg: '' };
  }

  update(patch: Partial<Local>) {
    this.state = { ...this.local, ...patch };
  }

  get meeting() {
    return this.local.fresh ?? this.load?.value;
  }

  get heading() {
    return this.meeting?.body_name || this.meeting?.title || 'Sitzung';
  }

  get meta() {
    const m = this.meeting;
    return [m?.date, m?.time, m?.location].filter(Boolean).join(' · ') || '—';
  }

  get publicText() {
    return publicLabel(this.meeting?.public).text;
  }

  get risUrl() {
    return `${RIS_BASE}/${this.args.id}`;
  }

  @cached
  get grouped() {
    const m = this.meeting;
    const agenda = m?.agenda_items ?? [];
    const known = new Set(agenda.map((a) => a.anchor));
    const byAnchor = new Map<string, Doc[]>();
    const extra: Doc[] = [];
    for (const f of m?.files ?? []) {
      const a = f.agenda_anchor;
      if (a && known.has(a)) byAnchor.set(a, [...(byAnchor.get(a) ?? []), f]);
      else extra.push(f);
    }
    const votes = new Map<string, Vote>();
    for (const v of m?.votes ?? [])
      if (v.agenda_anchor) votes.set(v.agenda_anchor, v);
    const items = agenda.map((a) => {
      const docs = byAnchor.get(a.anchor) ?? [];
      const vote = votes.get(a.anchor);
      return {
        ...a,
        // a new meeting gets new rows, so no TOP stays open from the last one
        key: `${m?.id}:${a.anchor}`,
        docs,
        vote,
        tally: vote && hasTally(vote) ? tally(vote) : '',
        nonPublic: a.public === 0 ? publicLabel(0).text : '',
        disabled: docs.length === 0 && !vote,
      };
    });
    return { items, extra };
  }

  get noFiles() {
    return (this.meeting?.files ?? []).length === 0;
  }

  openDoc = (doc: Doc) => this.nav.open({ meeting: '', doc: doc.id });

  refresh = async () => {
    const id = this.args.id;
    this.update({ busy: true, msg: '' });
    try {
      const fresh = await refreshMeeting(id);
      const c = fresh._refresh?.counts || {};
      const added = (c.files_new || 0) + (c.files_updated || 0);
      if (this.local.id !== id) return;
      this.update({
        fresh,
        busy: false,
        msg:
          added > 0
            ? `${added} neue/aktualisierte Dokument(e) übernommen.`
            : 'Keine neuen Dokumente. Analyse folgt beim nächsten Lauf.',
      });
    } catch (e) {
      if (this.local.id !== id) return;
      this.update({
        busy: false,
        msg: `Aktualisierung fehlgeschlagen: ${e instanceof Error ? e.message : String(e)}`,
      });
    }
  };

  <template>
    <Modal @heading={{this.heading}} @open={{@open}} @onClose={{@onClose}}>
      <LoadState @load={{this.load}}>
        <div class="dis-meeting">
          <div class="dis-meeting-head">
            <p class="dis-meeting-meta">{{this.meta}}</p>
            <Tag>{{this.publicText}}</Tag>
            <a href={{this.risUrl}} target="_blank" rel="noopener noreferrer">
              Sitzung im RIS öffnen
              <Icon @name="launch" @size={{12}} />
            </a>
          </div>

          <div class="dis-meeting-actions">
            {{#if this.local.busy}}
              <InlineLoading @text="Aktualisiere aus dem RIS…" />
            {{else}}
              <CanonButton
                class="dis-meeting-refresh"
                @kind="cell"
                @text="Aktualisieren"
                @icon={{component Icon name="renew"}}
                @onClick={{this.refresh}}
              />
            {{/if}}
            {{#if this.local.msg}}
              <span
                class="dis-meeting-msg"
                role="status"
              >{{this.local.msg}}</span>
            {{/if}}
          </div>

          <CanonRows @framed={{true}}>
            <h4 class="canon-heading">Tagesordnung ({{num
                this.grouped.items.length
              }})</h4>
            {{#each this.grouped.items key="key" as |a|}}
              {{#if a.disabled}}
                <div class="dis-meeting-top">
                  <span class="dis-meeting-topnum">{{if
                      a.number
                      (concatTop a.number)
                      "TOP"
                    }}</span>
                  <span class="dis-meeting-toptext">{{if
                      a.title
                      a.title
                      "—"
                    }}</span>
                  {{#if a.nonPublic}}
                    <span class="dis-meeting-topmeta"><Tag
                      >{{a.nonPublic}}</Tag></span>
                  {{/if}}
                </div>
              {{else}}
                <details class="dis-meeting-item">
                  <summary class="dis-meeting-top">
                    <Icon @name="chevron-right" class="dis-meeting-arrow" />
                    <span class="dis-meeting-topnum">{{if
                        a.number
                        (concatTop a.number)
                        "TOP"
                      }}</span>
                    <span class="dis-meeting-toptext">{{if
                        a.title
                        a.title
                        "—"
                      }}</span>
                    <span class="dis-meeting-topmeta">
                      {{#if a.tally}}
                        <Tag title="Ja · Nein · Enthaltung">{{a.tally}}</Tag>
                      {{/if}}
                      {{#if a.docs.length}}
                        <Tag>{{num a.docs.length}} Dok.</Tag>
                      {{/if}}
                      {{#if a.nonPublic}}
                        <Tag>{{a.nonPublic}}</Tag>
                      {{/if}}
                    </span>
                  </summary>
                  <div class="dis-meeting-body">
                    {{#if a.vote}}<VoteBlock @vote={{a.vote}} />{{/if}}
                    {{#each a.docs key="id" as |f|}}
                      <DocRow
                        @doc={{f}}
                        @submitters={{true}}
                        @onOpen={{fn this.openDoc f}}
                      />
                    {{/each}}
                  </div>
                </details>
              {{/if}}
            {{else}}
              <p class="dis-meeting-note">Keine Tagesordnung erfasst.</p>
            {{/each}}
          </CanonRows>

          {{#if this.grouped.extra.length}}
            <CanonRows @framed={{true}}>
              <h4 class="canon-heading">Weitere Dokumente ({{num
                  this.grouped.extra.length
                }})</h4>
              <div>
                {{#each this.grouped.extra key="id" as |f|}}
                  <DocRow
                    @doc={{f}}
                    @submitters={{true}}
                    @onOpen={{fn this.openDoc f}}
                  />
                {{/each}}
              </div>
            </CanonRows>
          {{/if}}

          {{#if this.noFiles}}
            <p class="dis-meeting-note">Keine Dokumente zu dieser Sitzung.</p>
          {{/if}}
        </div>
      </LoadState>
    </Modal>
  </template>
}
