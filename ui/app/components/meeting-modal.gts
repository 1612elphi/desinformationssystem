import Component from '@glimmer/component';
import { tracked, cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import {
  getMeeting,
  refreshMeeting,
  type Doc,
  type Meeting,
  type Vote,
} from 'ui/api';
import DocRow from 'ui/components/doc-row';
import Icon from 'ui/components/icon';
import { InlineLoading } from 'ui/components/loading';
import Modal from 'ui/components/modal';
import Tag from 'ui/components/tag';
import VoteBlock from 'ui/components/vote-block';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { RIS_BASE, hasTally, publicLabel, tally } from 'ui/utils/meta';

const concatTop = (n: string) => `TOP ${n}`;

interface Signature {
  Args: { id: string; onClose: () => void };
}

// State that belongs to one meeting id and resets when another one opens.
interface Local {
  id: string;
  fresh?: Meeting;
  busy: boolean;
  msg: string;
  open: string[];
}

export default class MeetingModal extends Component<Signature> {
  @service declare nav: NavService;
  @tracked state: Local = { id: '', busy: false, msg: '', open: [] };

  @cached
  get load() {
    return new Load(getMeeting(this.args.id));
  }

  get local(): Local {
    return this.state.id === this.args.id
      ? this.state
      : { id: this.args.id, busy: false, msg: '', open: [] };
  }

  update(patch: Partial<Local>) {
    this.state = { ...this.local, ...patch };
  }

  get meeting() {
    return this.local.fresh ?? this.load.value;
  }

  get heading() {
    return this.meeting?.body_name || this.meeting?.title || 'Sitzung';
  }

  get meta() {
    const m = this.meeting;
    return [m?.date, m?.time, m?.location].filter(Boolean).join(' · ') || '—';
  }

  get publicTag() {
    return publicLabel(this.meeting?.public);
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
        docs,
        vote,
        tally: vote && hasTally(vote) ? tally(vote) : '',
        nonPublic: a.public === 0 ? publicLabel(0) : undefined,
        disabled: docs.length === 0 && !vote,
      };
    });
    return { items, extra };
  }

  get noFiles() {
    return (this.meeting?.files ?? []).length === 0;
  }

  isOpen = (anchor: string) => this.local.open.includes(anchor);

  toggle = (anchor: string) => {
    const open = this.local.open;
    this.update({
      open: open.includes(anchor)
        ? open.filter((a) => a !== anchor)
        : [...open, anchor],
    });
  };

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
    {{#if this.meeting}}
      <Modal @heading={{this.heading}} @onClose={{@onClose}}>
        <div class="dis-detail dis-meeting">
          <div class="dis-meeting__head">
            <p class="dis-meeting__meta">{{this.meta}}</p>
            <div class="dis-meeting__headtags">
              <Tag
                @type={{this.publicTag.type}}
                @size="sm"
              >{{this.publicTag.text}}</Tag>
              <a
                class="cds--link"
                href={{this.risUrl}}
                target="_blank"
                rel="noopener noreferrer"
              >
                Sitzung im RIS öffnen
                <Icon @name="launch" @size={{12}} />
              </a>
            </div>
          </div>

          <div class="dis-meeting__actions">
            {{#if this.local.busy}}
              <InlineLoading @text="Aktualisiere aus dem RIS…" />
            {{else}}
              <button
                type="button"
                class="cds--btn cds--btn--tertiary cds--btn--sm"
                {{on "click" this.refresh}}
              >
                Aktualisieren
                <Icon @name="renew" class="cds--btn__icon" />
              </button>
            {{/if}}
            {{#if this.local.msg}}
              <span class="dis-meta-line">{{this.local.msg}}</span>
            {{/if}}
          </div>

          <h4>Tagesordnung ({{this.grouped.items.length}})</h4>
          {{#if this.grouped.items.length}}
            <div class="dis-meeting__agenda">
              <ul class="cds--accordion cds--accordion--end cds--accordion--sm">
                {{#each this.grouped.items key="anchor" as |a|}}
                  <li
                    class="cds--accordion__item
                      {{if
                        (this.isOpen a.anchor)
                        'cds--accordion__item--active'
                      }}
                      {{if a.disabled 'cds--accordion__item--disabled'}}"
                  >
                    <button
                      type="button"
                      class="cds--accordion__heading"
                      aria-expanded={{if (this.isOpen a.anchor) "true" "false"}}
                      disabled={{a.disabled}}
                      {{on "click" (fn this.toggle a.anchor)}}
                    >
                      <Icon
                        @name="chevron-right"
                        class="cds--accordion__arrow"
                      />
                      <div class="cds--accordion__title">
                        <span class="dis-meeting__topttl">
                          <span class="dis-meeting__topnum">{{if
                              a.number
                              (concatTop a.number)
                              "TOP"
                            }}</span>
                          <span class="dis-meeting__toptext">{{if
                              a.title
                              a.title
                              "—"
                            }}</span>
                          <span class="dis-meeting__topmeta">
                            {{#if a.tally}}
                              <Tag
                                @type="green"
                                @size="sm"
                                title="Ja · Nein · Enthaltung"
                              >{{a.tally}}</Tag>
                            {{/if}}
                            {{#if a.docs.length}}
                              <Tag @type="outline" @size="sm">{{a.docs.length}}
                                Dok.</Tag>
                            {{/if}}
                            {{#if a.nonPublic}}
                              <Tag
                                @type={{a.nonPublic.type}}
                                @size="sm"
                              >{{a.nonPublic.text}}</Tag>
                            {{/if}}
                          </span>
                        </span>
                      </div>
                    </button>
                    {{#if (this.isOpen a.anchor)}}
                      <div class="cds--accordion__wrapper">
                        <div class="cds--accordion__content">
                          {{#if a.vote}}<VoteBlock @vote={{a.vote}} />{{/if}}
                          <div class="dis-meeting__docs">
                            {{#each a.docs key="id" as |f|}}
                              <DocRow
                                @doc={{f}}
                                @submitters={{true}}
                                @onOpen={{fn this.openDoc f}}
                              />
                            {{/each}}
                          </div>
                        </div>
                      </div>
                    {{/if}}
                  </li>
                {{/each}}
              </ul>
            </div>
          {{else}}
            <p class="dis-meta-line">Keine Tagesordnung erfasst.</p>
          {{/if}}

          {{#if this.grouped.extra.length}}
            <h4>Weitere Dokumente ({{this.grouped.extra.length}})</h4>
            <div class="dis-meeting__docs">
              {{#each this.grouped.extra key="id" as |f|}}
                <DocRow
                  @doc={{f}}
                  @submitters={{true}}
                  @onOpen={{fn this.openDoc f}}
                />
              {{/each}}
            </div>
          {{/if}}

          {{#if this.noFiles}}
            <p class="dis-meta-line dis-spaced">Keine Dokumente zu dieser
              Sitzung.</p>
          {{/if}}
        </div>
      </Modal>
    {{/if}}
  </template>
}
