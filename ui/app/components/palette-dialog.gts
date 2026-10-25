import Component from '@glimmer/component';
import { cached, tracked } from '@glimmer/tracking';
import { fn } from '@ember/helper';
import { service } from '@ember/service';
import { htmlSafe } from '@ember/template';
import type RouterService from '@ember/routing/router-service';
import type { ComponentLike } from '@glint/template';
import CanonCommand, {
  type CanonCommandItem,
} from 'delphicanon/components/canon-command';
import CanonDialog, {
  type CanonDialogClose,
} from 'delphicanon/components/canon-dialog';
import { itemScore } from 'delphicanon/lib/command';
import {
  getMeetings,
  search,
  type Doc,
  type Meeting,
  type SearchResult,
} from 'ui/api';
import Icon from 'ui/components/icon';
import { LoadError } from 'ui/components/loading';
import type NavService from 'ui/services/nav';
import { getCommitteeList, getPeople } from 'ui/utils/api-shell';
import Load from 'ui/utils/load';
import './palette-dialog.css';

export const VIEWS = [
  { route: 'index', label: 'Dokumente', icon: 'document', key: 'd' },
  { route: 'sitzungen', label: 'Sitzungen', icon: 'calendar', key: 's' },
  { route: 'gremien', label: 'Gremien', icon: 'users', key: 'g' },
  { route: 'statistik', label: 'Statistik', icon: 'chart', key: 't' },
  { route: 'reden', label: 'Reden', icon: 'mic', key: 'r' },
  { route: 'karte', label: 'Karte', icon: 'map', key: 'k' },
];

type Kind = 'view' | 'doc' | 'meeting' | 'person' | 'committee';

const ICONS: Record<Kind, ComponentLike> = {
  view: <template><Icon @name="chevron-right" /></template>,
  doc: <template><Icon @name="document" /></template>,
  meeting: <template><Icon @name="calendar" /></template>,
  person: <template><Icon @name="users" /></template>,
  committee: <template><Icon @name="users" /></template>,
};

const iconFor = (name: string): ComponentLike => <template>
  <Icon @name={{name}} />
</template>;
const VIEW_ICONS = Object.fromEntries(
  VIEWS.map((v) => [v.route, iconFor(v.icon)]),
);

const RECENT_KEY = 'dis-palette-recent';
const RECENT_MAX = 5;
const PER_GROUP = 6;
const DOC_DELAY = 250;
const style = htmlSafe('--canon-dialog-width: 640px');

interface Stored {
  id: string;
  title: string;
  detail?: string;
}

const kindOf = (id: string) => id.slice(0, id.indexOf(':')) as Kind;
const targetOf = (id: string) => id.slice(id.indexOf(':') + 1);
const item = (s: Stored, group: string, more?: Partial<CanonCommandItem>) => ({
  ...s,
  group,
  icon: ICONS[kindOf(s.id)],
  ...more,
});
const dateDe = (iso?: string) =>
  iso ? iso.split('-').reverse().join('.') : '';

function readRecent(): Stored[] {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
    if (!Array.isArray(v)) return [];
    return (v as Stored[]).filter(
      (s) => typeof s?.id === 'string' && typeof s.title === 'string',
    );
  } catch {
    return [];
  }
}

// best matches first, capped; the command list itself re-ranks across groups
const top = (items: CanonCommandItem[], q: string) =>
  items
    .map((i) => ({ i, s: itemScore(q, i) }))
    .filter((x) => x.s !== undefined)
    .sort((a, b) => (b.s ?? 0) - (a.s ?? 0))
    .slice(0, PER_GROUP)
    .map((x) => x.i);

const meetingItem = (m: Meeting) =>
  item(
    {
      id: `meeting:${m.id}`,
      title: m.body_name || m.title || m.id,
      detail: [dateDe(m.date), m.location].filter(Boolean).join(' · '),
    },
    'Sitzungen',
    { keywords: [m.date ?? '', dateDe(m.date), m.title ?? ''] },
  );

interface Signature {
  Args: { open: boolean; onClose: () => void };
}

export default class PaletteDialog extends Component<Signature> {
  @service declare router: RouterService;
  @service declare nav: NavService;

  @tracked query = '';
  @tracked docQuery = '';
  @tracked recent = readRecent();
  #picked?: CanonCommandItem;
  #timer?: ReturnType<typeof setTimeout>;

  #data?: ReturnType<PaletteDialog['load']>;

  // fetched on the first open, kept for the session
  get data() {
    // untracked cache, written once
    // eslint-disable-next-line ember/no-side-effects
    if (!this.#data && this.args.open) this.#data = this.load();
    return this.#data;
  }

  load() {
    const today = new Date().toISOString().slice(0, 10);
    return {
      people: new Load(getPeople()),
      committees: new Load(getCommitteeList()),
      upcoming: new Load(getMeetings({ upcoming: true, limit: 5 })),
      past: new Load(getMeetings({ to: today, limit: 5 })),
    };
  }

  #lastDocs?: Load<SearchResult>;

  @cached
  get docs() {
    if (this.docQuery.length < 2) return undefined;
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#lastDocs = new Load(
      search({ q: this.docQuery, limit: PER_GROUP }),
      this.#lastDocs,
    ));
  }

  get items(): CanonCommandItem[] {
    const q = this.query.trim();
    const d = this.data;
    const views = VIEWS.map((v) =>
      item({ id: `view:${v.route}`, title: v.label }, 'Ansichten', {
        shortcut: ['G', v.key.toUpperCase()],
        icon: VIEW_ICONS[v.route],
      }),
    );
    const meetings = [
      ...(d?.upcoming.value?.meetings ?? []),
      ...(d?.past.value?.meetings ?? []),
    ].map(meetingItem);
    if (!q)
      return [
        ...this.recent.map((s) => item(s, 'Zuletzt')),
        ...views,
        ...meetings,
      ];
    const people = (d?.people.value ?? []).map((p) =>
      item(
        { id: `person:${p.person_id}`, title: p.name, detail: p.party },
        'Personen',
        { keywords: p.party_code ? [p.party_code] : [] },
      ),
    );
    const committees = (d?.committees.value ?? []).map((c) =>
      item({ id: `committee:${c.id}`, title: c.name }, 'Gremien'),
    );
    const docs = (this.docs?.value?.results ?? []).map((doc) =>
      item({ id: `doc:${doc.id}`, title: doc.label }, 'Dokumente', {
        // full-text hits always match; the prefix scores them as a substring match, under name matches
        keywords: [`…${q}`],
        detail: this.docDetail(doc),
      }),
    );
    return [
      ...views,
      ...docs,
      ...top(meetings, q),
      ...top(people, q),
      ...top(committees, q),
    ];
  }

  get error() {
    const d = this.data;
    return [d?.people, d?.committees, d?.upcoming, d?.past, this.docs].find(
      (l) => l?.error,
    )?.error;
  }

  docDetail = (doc: Doc) =>
    [doc.doc_type, doc.body_name, dateDe(doc.meeting_date)]
      .filter(Boolean)
      .join(' · ');

  setQuery = (q: string) => {
    this.query = q;
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => (this.docQuery = q.trim()), DOC_DELAY);
  };

  pick = (close: CanonDialogClose, picked: CanonCommandItem) => {
    this.#picked = picked;
    close(picked.id);
  };

  // runs after the dialog has closed, so a sheet opened here is the only dialog
  closed = () => {
    clearTimeout(this.#timer);
    this.query = '';
    this.docQuery = '';
    this.args.onClose();
    const picked = this.#picked;
    this.#picked = undefined;
    if (picked) this.go(picked);
  };

  go(picked: CanonCommandItem) {
    const target = targetOf(picked.id);
    const kind = kindOf(picked.id);
    if (kind !== 'view') this.remember(picked);
    if (kind === 'view') this.router.transitionTo(target);
    else if (kind === 'committee')
      this.router.transitionTo('gremien', {
        queryParams: { gremium: target },
      });
    else if (kind === 'doc') this.nav.open({ doc: target });
    else if (kind === 'meeting') this.nav.open({ meeting: target });
    else if (kind === 'person') this.nav.open({ person: target });
  }

  remember({ id, title, detail }: CanonCommandItem) {
    this.recent = [
      { id, title, detail },
      ...this.recent.filter((s) => s.id !== id),
    ].slice(0, RECENT_MAX);
    try {
      localStorage.setItem(RECENT_KEY, JSON.stringify(this.recent));
    } catch {
      // storage blocked: the list lasts for this page view only
    }
  }

  <template>
    <CanonDialog
      class="dis-palette"
      @open={{@open}}
      @onClose={{this.closed}}
      aria-label="Befehle"
      style={{style}}
    >
      <:default as |close|>
        <CanonCommand
          @items={{this.items}}
          @label="Suche"
          @placeholder="Suchen…"
          @empty="Keine Treffer"
          @onSelect={{fn this.pick close}}
          @onQuery={{this.setQuery}}
        />
        {{#if this.error}}
          <LoadError @error={{this.error}} />
        {{/if}}
      </:default>
    </CanonDialog>
  </template>
}
