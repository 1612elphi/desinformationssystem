import Component from '@glimmer/component';
import { tracked, cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import type RouterService from '@ember/routing/router-service';
import CanonButton from 'delphicanon/components/canon-button';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonSegmented from 'delphicanon/components/canon-segmented';
import CanonTiles from 'delphicanon/components/canon-tiles';
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
import MeetingTop, { type TopItem } from 'ui/components/meeting-top';
import Modal, { type PaneContext } from 'ui/components/modal';
import SpeechList from 'ui/components/speech-list';
import Tag from 'ui/components/tag';
import { outcome } from 'ui/components/vote-block';
import rough from 'ui/modifiers/rough';
import shortcut from 'ui/modifiers/shortcut';
import type NavService from 'ui/services/nav';
import { getPersonIds, getSpeeches, type Speech } from 'ui/utils/api-reden';
import { getNeighbours } from 'ui/utils/api-sitzung';
import { isLive } from 'ui/utils/calendar';
import Load from 'ui/utils/load';
import { RIS_BASE, num, publicLabel } from 'ui/utils/meta';
import './meeting-modal.css';

const FILTERS = ['Alle', 'Abstimmungen', 'Strittig', 'Dokumente', 'Reden'];
const MATCH: ((i: TopItem) => boolean)[] = [
  () => true,
  (i) => !!i.vote,
  (i) => i.kind === 'contested',
  (i) => i.docs.length > 0,
  (i) => i.speechCount > 0,
];

const SPEECH_NOTE = 'Gedächtnisprotokoll, hihi';

type SpeechMeeting = Meeting & { speech_counts?: Record<string, number> };

const parts = (n?: string) => (n ?? '').split('.').map((x) => Number(x));
const numeric = (n?: string) => !!n && parts(n).every(Number.isInteger);
const byNumber = (a?: string, b?: string) => {
  const x = parts(a);
  const y = parts(b);
  for (let i = 0; i < Math.max(x.length, y.length); i++) {
    const d = (x[i] ?? -1) - (y[i] ?? -1);
    if (d) return d;
  }
  return 0;
};

const asDate = (d: string) => new Date(`${d}T12:00:00`);
const shortDate = (d?: string) =>
  d ? asDate(d).toLocaleDateString('de-DE', { dateStyle: 'medium' }) : '';
const eq = (a: unknown, b: unknown) => a === b;

interface Signature {
  Args: { id: string; onClose: () => void; pane?: PaneContext };
}

// State that belongs to one meeting id and resets when another one opens.
interface Local {
  id: string;
  fresh?: Meeting;
  busy: boolean;
  msg: string;
  filter: number;
  open: ReadonlySet<string>;
  cursor: number;
}

const blank = (id: string): Local => ({
  id,
  busy: false,
  msg: '',
  filter: 0,
  open: new Set(),
  cursor: -1,
});

export default class MeetingModal extends Component<Signature> {
  @service declare nav: NavService;
  @service declare router: RouterService;
  @tracked state: Local = blank('');

  @cached
  get load() {
    return this.args.id ? new Load(getMeeting(this.args.id)) : undefined;
  }

  get speechCounts() {
    return this.meeting?.speech_counts ?? {};
  }

  get hasSpeeches() {
    return Object.keys(this.speechCounts).length > 0;
  }

  get filters() {
    return this.hasSpeeches ? FILTERS : FILTERS.slice(0, -1);
  }

  @cached
  get speechLoad() {
    const id = this.meeting?.id;
    return id && this.hasSpeeches ? new Load(getSpeeches(id)) : undefined;
  }

  personIds = new Load(getPersonIds());

  @cached
  get neighbours() {
    const m = this.load?.value;
    return m ? new Load(getNeighbours(m)) : undefined;
  }

  get prev() {
    return this.neighbours?.value?.prev;
  }

  get next() {
    return this.neighbours?.value?.next;
  }

  get local(): Local {
    return this.state.id === this.args.id ? this.state : blank(this.args.id);
  }

  update(patch: Partial<Local>) {
    this.state = { ...this.local, ...patch };
  }

  get meeting(): SpeechMeeting | undefined {
    return this.local.fresh ?? this.load?.value;
  }

  get heading() {
    return this.meeting?.body_name || this.meeting?.title || 'Sitzung';
  }

  @cached
  get date() {
    const d = this.meeting?.date;
    if (!d) return undefined;
    const t = asDate(d);
    return {
      iso: d,
      weekday: t.toLocaleDateString('de-DE', { weekday: 'long' }),
      day: t.getDate(),
      month: t.toLocaleDateString('de-DE', { month: 'long' }),
      year: t.getFullYear(),
    };
  }

  get publicText() {
    return publicLabel(this.meeting?.public).text;
  }

  get publicIcon() {
    const p = this.meeting?.public;
    return p === 1 ? 'globe' : p === 0 ? 'lock' : undefined;
  }

  get live() {
    return !!this.meeting && isLive(this.meeting);
  }

  get risUrl() {
    return `${RIS_BASE}/${this.args.id}`;
  }

  @cached
  get grouped() {
    const m = this.meeting;
    const agenda = [...(m?.agenda_items ?? [])];
    if (agenda.every((a) => numeric(a.number)))
      agenda.sort((a, b) => byNumber(a.number, b.number));
    const known = new Set(agenda.map((a) => a.anchor));
    const byAnchor = new Map<string, Doc[]>();
    const extra: Doc[] = [];
    for (const f of m?.files ?? []) {
      const a = f.agenda_anchor;
      if (a && known.has(a)) byAnchor.set(a, [...(byAnchor.get(a) ?? []), f]);
      else extra.push(f);
    }
    const speeches = new Map<string, Speech[]>();
    for (const sp of this.speechLoad?.value ?? []) {
      const a =
        sp.agenda_anchor && known.has(sp.agenda_anchor) ? sp.agenda_anchor : '';
      speeches.set(a, [...(speeches.get(a) ?? []), sp]);
    }
    const counts = this.speechCounts;
    const votes = new Map<string, Vote>();
    for (const v of m?.votes ?? [])
      if (v.agenda_anchor) votes.set(v.agenda_anchor, v);
    const items = agenda.map((a, i): TopItem => {
      const docs = byAnchor.get(a.anchor) ?? [];
      const vote = votes.get(a.anchor);
      const result = outcome(vote);
      const speechCount = counts[a.anchor] ?? 0;
      return {
        key: `${m?.id}:${a.anchor}`,
        dom: `dis-mtop-${i}`,
        number: a.number,
        title: a.title,
        sub: !!a.number?.includes('.'),
        docs,
        vote,
        outcome: result,
        speeches: speeches.get(a.anchor) ?? [],
        speechCount,
        nonPublic: a.public === 0 ? publicLabel(0).text : '',
        withdrawn: /abgesetzt/i.test(a.title ?? ''),
        expandable: docs.length > 0 || !!vote || speechCount > 0,
        kind: result?.contested
          ? 'contested'
          : vote
            ? 'vote'
            : docs.length
              ? 'docs'
              : 'none',
      };
    });
    return { items, extra, speeches: speeches.get('') ?? [] };
  }

  get tiles() {
    const items = this.grouped.items;
    const votes = items.filter((i) => i.vote).length;
    const contested = items.filter((i) => i.kind === 'contested').length;
    return [
      { label: 'TOPs', value: num(items.length) },
      { label: 'Abstimmungen', value: num(votes) },
      {
        label: 'Strittig',
        value: num(contested),
        level: votes ? contested / votes : undefined,
      },
      { label: 'Dokumente', value: num(this.meeting?.files?.length) },
      ...(this.hasSpeeches
        ? [
            {
              label: 'Wortbeiträge',
              value: num(
                Object.values(this.speechCounts).reduce((a, n) => a + n, 0),
              ),
            },
          ]
        : []),
    ];
  }

  get visible() {
    return this.grouped.items.filter(MATCH[this.local.filter] ?? MATCH[0]!);
  }

  get allOpen() {
    const open = this.local.open;
    const xs = this.visible.filter((i) => i.expandable);
    return xs.length > 0 && xs.every((i) => open.has(i.key));
  }

  get noFiles() {
    return (this.meeting?.files ?? []).length === 0;
  }

  isOpen = (key: string) => this.local.open.has(key);

  toggle = (key: string) => {
    const open = new Set(this.local.open);
    if (!open.delete(key)) open.add(key);
    this.update({ open });
  };

  toggleAll = () => {
    this.update({
      open: this.allOpen
        ? new Set()
        : new Set(this.visible.filter((i) => i.expandable).map((i) => i.key)),
    });
  };

  setFilter = (filter: number) => this.update({ filter, cursor: -1 });

  cycleFilter = () =>
    this.setFilter((this.local.filter + 1) % this.filters.length);

  setCursor = (cursor: number) => {
    if (cursor !== this.local.cursor) this.update({ cursor });
  };

  focusTop = (i: number) => {
    const item = this.visible[i];
    if (!item) return;
    this.setCursor(i);
    const el = document.getElementById(`${item.dom}-head`);
    el?.focus({ preventScroll: true });
    el?.closest('li')?.scrollIntoView({ block: 'nearest' });
  };

  step = (by: number) => () => {
    const n = this.visible.length;
    if (n) this.focusTop(Math.min(n - 1, Math.max(0, this.local.cursor + by)));
  };

  goto = (m?: Meeting) => {
    if (!m) return;
    document.getElementById('dis-meeting')?.scrollIntoView();
    this.nav.replaceTop('meeting', m.id);
  };

  gotoPrev = () => this.goto(this.prev);
  gotoNext = () => this.goto(this.next);

  openDoc = (doc: Doc) => this.nav.open({ doc: doc.id });
  openPerson = (id: string) => this.nav.open({ person: id });

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
    <Modal @heading={{this.heading}} @pane={{@pane}} @onClose={{@onClose}}>
      <LoadState @load={{this.load}}>
        <div
          class="dis-meeting"
          id="dis-meeting"
          {{shortcut "j" (this.step 1)}}
          {{shortcut "k" (this.step -1)}}
          {{shortcut "e" this.toggleAll}}
          {{shortcut "f" this.cycleFilter}}
          {{shortcut "[" this.gotoPrev}}
          {{shortcut "]" this.gotoNext}}
          {{shortcut "h" this.gotoPrev}}
          {{shortcut "l" this.gotoNext}}
        >
          <header class="dis-mhead">
            {{#if this.date}}
              <time class="dis-mdate" datetime={{this.date.iso}}>
                <span class="dis-mdate-wd">{{this.date.weekday}}</span>
                {{! static branches, so J/K and the filter do not redraw the mark }}
                {{#if this.live}}
                  <span
                    class="dis-mdate-day"
                    {{rough "circle" padding=2 delay=300}}
                  >{{this.date.day}}</span>
                {{else}}
                  <span
                    class="dis-mdate-day"
                    {{rough "underline" strokeWidth=2.5 padding=0 delay=300}}
                  >{{this.date.day}}</span>
                {{/if}}
                <span class="dis-mdate-mon">{{this.date.month}}
                  {{this.date.year}}</span>
              </time>
            {{/if}}
            <div class="dis-mhead-main">
              {{#if this.meeting.title}}
                <p class="dis-mhead-body">{{this.meeting.title}}</p>
              {{/if}}
              <p class="dis-mhead-facts">
                {{#if this.meeting.time}}
                  <span><Icon
                      @name="time"
                      @size={{14}}
                    />{{this.meeting.time}}</span>
                {{/if}}
                {{#if this.meeting.location}}
                  <span><Icon
                      @name="map"
                      @size={{14}}
                    />{{this.meeting.location}}</span>
                {{/if}}
                <Tag>{{#if this.publicIcon}}<Icon
                      @name={{this.publicIcon}}
                      @size={{12}}
                    />{{/if}}{{this.publicText}}</Tag>
              </p>
              <div class="dis-mhead-actions">
                <a
                  class="dis-mhead-ris"
                  href={{this.risUrl}}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Sitzung im RIS öffnen
                  <Icon @name="launch" @size={{12}} />
                </a>
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
            </div>
            <nav class="dis-mhead-nav" aria-label="Sitzungen">
              {{#if this.prev}}
                <CanonButton
                  class="dis-mhead-step"
                  @kind="cell"
                  @text={{shortDate this.prev.date}}
                  @label="Vorige Sitzung"
                  @icon={{component Icon name="chevron-left"}}
                  @onClick={{this.gotoPrev}}
                  aria-keyshortcuts="["
                />
              {{/if}}
              {{#if this.next}}
                <CanonButton
                  class="dis-mhead-step dis-mhead-step--next"
                  @kind="cell"
                  @text={{shortDate this.next.date}}
                  @label="Nächste Sitzung"
                  @icon={{component Icon name="chevron-right"}}
                  @onClick={{this.gotoNext}}
                  aria-keyshortcuts="]"
                />
              {{/if}}
            </nav>
          </header>

          <CanonTiles class="dis-meeting-tiles" @items={{this.tiles}} />

          <div class="dis-meeting-bar">
            <div class="dis-meeting-controls">
              <CanonSegmented
                class="dis-meeting-filter"
                @items={{this.filters}}
                @selected={{this.local.filter}}
                @onChange={{this.setFilter}}
                aria-keyshortcuts="f"
              />
              <CanonButton
                class="dis-meeting-all"
                @kind="cell"
                @text={{if this.allOpen "Alle zuklappen" "Alle aufklappen"}}
                @onClick={{this.toggleAll}}
                aria-keyshortcuts="e"
              />
              <p class="dis-meeting-keys" aria-hidden="true">
                <kbd>J</kbd><kbd>K</kbd>
                TOP ·
                <kbd>E</kbd>
                Alle ·
                <kbd>F</kbd>
                Filter ·
                <kbd>[</kbd><kbd>]</kbd>
                Sitzung
              </p>
            </div>
            {{#if this.visible.length}}
              <nav class="dis-meeting-index" aria-label="Sprungmarken">
                {{#each this.visible key="key" as |a i|}}
                  <button
                    type="button"
                    class="dis-meeting-index-n dis-meeting-index-n--{{a.kind}}
                      {{if
                        (eq i this.local.cursor)
                        'dis-meeting-index-n--current'
                      }}"
                    aria-label={{if a.number a.number "TOP"}}
                    {{on "click" (fn this.focusTop i)}}
                  >{{if a.number a.number "·"}}</button>
                {{/each}}
              </nav>
            {{/if}}
          </div>

          <section class="dis-meeting-agenda" aria-label="Tagesordnung">
            <h4 class="canon-heading"><Icon @name="list-ordered" />Tagesordnung
              ({{num this.visible.length}})</h4>
            {{#if this.hasSpeeches}}
              <p class="dis-meeting-note">{{SPEECH_NOTE}}</p>
            {{/if}}
            <ol class="dis-meeting-timeline">
              {{#each this.visible key="key" as |a i|}}
                <MeetingTop
                  @item={{a}}
                  @open={{this.isOpen a.key}}
                  @current={{eq i this.local.cursor}}
                  @onToggle={{fn this.toggle a.key}}
                  @onFocus={{fn this.setCursor i}}
                  @onOpenDoc={{this.openDoc}}
                  @speechLoad={{this.speechLoad}}
                  @personIds={{this.personIds.value}}
                  @onOpenPerson={{this.openPerson}}
                />
              {{else}}
                <li class="dis-meeting-note">{{if
                    this.grouped.items.length
                    "Keine Treffer"
                    "Keine Tagesordnung erfasst."
                  }}</li>
              {{/each}}
            </ol>
          </section>

          {{#if this.grouped.speeches.length}}
            <section class="dis-meeting-more">
              <h4 class="canon-heading"><Icon @name="mic" />Weitere Wortbeiträge</h4>
              <SpeechList
                @speeches={{this.grouped.speeches}}
                @personIds={{this.personIds.value}}
                @onOpenPerson={{this.openPerson}}
              />
            </section>
          {{/if}}

          {{#if this.grouped.extra.length}}
            <CanonRows @framed={{true}}>
              <h4 class="canon-heading"><Icon @name="files" />Weitere Dokumente
                ({{num this.grouped.extra.length}})</h4>
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
            <p class="dis-meeting-note"><Icon @name="file-x" @size={{14}} />
              Keine Dokumente zu dieser Sitzung.</p>
          {{/if}}
        </div>
      </LoadState>
    </Modal>
  </template>
}
