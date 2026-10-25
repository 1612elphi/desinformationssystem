import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import type { Meeting } from 'ui/api';
import { searchFaceted, type FacetedResult } from 'ui/utils/api-dokumente';
import {
  fold,
  matches,
  searchMeetings,
  searchSpeeches,
  searchVotes,
  type Hits,
  type PersonHit,
  type SpeechHit,
  type VoteHit,
} from 'ui/utils/api-suche';
import { getCommitteeList, getPeople } from 'ui/utils/api-shell';
import type { Column } from 'ui/components/data-table';
import { partyName } from 'ui/utils/api-menschen';
import Load from 'ui/utils/load';
import { PUBLIC_OPTS } from 'ui/utils/meta';

export const PAGE_SIZE = 25;

export const ALL_COLUMNS: Column[] = [
  { key: 'date', label: 'Datum', width: '6.5rem' },
  { key: 'gremium', label: 'Gremium', width: '10%' },
  { key: 'top', label: 'TOP', width: '14%' },
  { key: 'sub', label: 'Einbringer', width: '6.5rem' },
  { key: 'typ', label: 'Typ', width: '10%' },
  { key: 'doc', label: 'Dokument' },
  { key: 'cat', label: 'Kategorie', width: '9%' },
  { key: 'stadtteil', label: 'Stadtteil', width: '8%' },
];
const COL_KEYS = ALL_COLUMNS.map((c) => c.key);
const COLS_STORAGE = 'dis-cols';
const VIEW_STORAGE = 'dis-docs-view';
export const VIEWS = ['Liste', 'Tabelle'];
const FILTERS_STORAGE = 'dis-suche-filters';

export const SCOPES = [
  { id: 'alles', label: 'Alles', icon: 'suche-all' },
  { id: 'dokumente', label: 'Dokumente', icon: 'suche-docs' },
  { id: 'sitzungen', label: 'Sitzungen', icon: 'suche-meetings' },
  { id: 'personen', label: 'Personen', icon: 'suche-people' },
  { id: 'gremien', label: 'Gremien', icon: 'suche-bodies' },
  { id: 'reden', label: 'Reden', icon: 'suche-speeches' },
  { id: 'abstimmungen', label: 'Abstimmungen', icon: 'suche-votes' },
] as const;
export type Scope = (typeof SCOPES)[number]['id'];
export const LIST_SIZE = 25;

// name matches first: starts with the query, then a word does, then anywhere
const rankBy = (q: string, name: string) => {
  const n = fold(name);
  const f = fold(q);
  return n.startsWith(f) ? 0 : n.includes(` ${f}`) ? 1 : n.includes(f) ? 2 : 3;
};

function stored(key: string): string {
  try {
    return localStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}

function store(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // storage blocked: the choice lasts for this page view only
  }
}

function storedCols(): string[] {
  try {
    const s: unknown = JSON.parse(stored(COLS_STORAGE) || 'null');
    const valid = Array.isArray(s)
      ? s.filter((k): k is string => COL_KEYS.includes(k as string))
      : [];
    if (valid.length) return valid;
  } catch {
    // unreadable storage: use the default columns
  }
  return COL_KEYS;
}

type FilterKey =
  | 'committee'
  | 'type'
  | 'cat'
  | 'sub'
  | 'pub'
  | 'from'
  | 'to'
  | 'topic'
  | 'district'
  | 'street';

export default class SucheController extends Controller {
  queryParams = [
    'scope',
    'q',
    'committee',
    'type',
    'cat',
    'sub',
    'pub',
    'from',
    'to',
    'topic',
    'district',
    'street',
    'page',
  ];

  @tracked q = '';
  @tracked scope: Scope = 'alles';
  @tracked listPage = 1;

  setScope = (scope: Scope) => {
    this.scope = scope;
    // a same-value write still invalidates, and every search reads listPage
    if (this.listPage !== 1) this.listPage = 1;
    this.cursor = -1;
  };

  setListPage = (page: number) => {
    this.listPage = page;
    this.cursor = -1;
  };

  @tracked filtersOpen = stored(FILTERS_STORAGE) === '1';

  toggleFilters = () => {
    this.filtersOpen = !this.filtersOpen;
    store(FILTERS_STORAGE, this.filtersOpen ? '1' : '');
  };
  @tracked committee = '';
  @tracked type = '';
  @tracked cat = '';
  @tracked sub = '';
  @tracked pub = 'all';
  @tracked from = '';
  @tracked to = '';
  @tracked topic = '';
  @tracked district = '';
  @tracked street = '';

  @tracked cols = storedCols();

  setCols = (keys: string[]) => {
    this.cols = keys;
    store(COLS_STORAGE, JSON.stringify(keys));
  };

  @tracked view = Math.max(0, VIEWS.indexOf(stored(VIEW_STORAGE)));

  setView = (i: number) => {
    this.view = i;
    store(VIEW_STORAGE, VIEWS[i] ?? '');
  };

  // highlighted result row for j/k; -1 for none
  @tracked cursor = -1;

  setCursor = (i: number) => {
    this.cursor = i;
  };

  get visibleColumns() {
    return ALL_COLUMNS.filter((c) => this.cols.includes(c.key));
  }

  // The search box updates the URL (and the search) 350 ms after typing stops.
  @tracked qDraft: string | null = null;
  #qTimer?: ReturnType<typeof setTimeout>;

  get qValue() {
    return this.qDraft ?? this.q;
  }

  setQ = (v: string) => {
    this.qDraft = v;
    clearTimeout(this.#qTimer);
    this.#qTimer = setTimeout(() => {
      this.q = v;
      this.qDraft = null;
      this.page = 1;
      this.listPage = 1;
      this.cursor = -1;
    }, 350);
  };

  clearQ = () => {
    clearTimeout(this.#qTimer);
    this.qDraft = null;
    this.q = '';
    this.page = 1;
    this.listPage = 1;
    this.cursor = -1;
  };

  setFilter = (key: FilterKey, value: string) => {
    this[key] = value;
    this.page = 1;
    this.cursor = -1;
  };

  resetAll = () => {
    this.clearQ();
    this.committee = '';
    this.type = '';
    this.cat = '';
    this.sub = '';
    this.pub = 'all';
    this.from = '';
    this.to = '';
    this.topic = '';
    this.district = '';
    this.street = '';
  };

  get params() {
    return {
      q: this.q,
      committee: this.committee,
      type: this.type,
      category: this.cat,
      public: this.pub,
      from: this.from,
      to: this.to,
      topic: this.topic,
      submitter: this.sub,
      district: this.district,
      street: this.street,
    };
  }

  @tracked page = 1;

  setPage = (page: number) => {
    this.page = page;
    this.cursor = -1;
  };

  #lastSearch?: Load<FacetedResult>;

  @cached
  get search() {
    const req = searchFaceted({
      ...this.params,
      facets: true,
      limit: PAGE_SIZE,
      offset: (this.page - 1) * PAGE_SIZE,
    });
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#lastSearch = new Load(req, this.#lastSearch));
  }

  get facetCounts() {
    return this.search.value?.facet_counts;
  }

  get results() {
    return this.search.value?.results ?? [];
  }

  get total() {
    return this.search.value?.total ?? 0;
  }

  get feedUrl() {
    const p = Object.entries({
      committee: this.committee,
      type: this.type,
      submitter: this.sub,
      topic: this.topic,
    }).filter(([, v]) => v);
    return `/api/feed.xml?${new URLSearchParams(p).toString()}`;
  }

  get activeFilters(): { k: FilterKey; label: string; clear: string }[] {
    const f: { k: FilterKey; label: string; clear: string }[] = [];
    const add = (k: FilterKey, label: string, clear = '') =>
      f.push({ k, label, clear });
    if (this.committee) add('committee', `Gremium: ${this.committee}`);
    if (this.type) add('type', `Typ: ${this.type}`);
    if (this.cat) add('cat', `Kategorie: ${this.cat}`);
    if (this.sub) add('sub', `Einbringer: ${partyName(this.sub)}`);
    if (this.pub !== 'all')
      add(
        'pub',
        PUBLIC_OPTS.find((o) => o.id === this.pub)?.label || this.pub,
        'all',
      );
    if (this.from) add('from', `ab ${this.from}`);
    if (this.to) add('to', `bis ${this.to}`);
    if (this.topic) add('topic', `Thema: ${this.topic}`);
    if (this.district) add('district', `Stadtteil: ${this.district}`);
    if (this.street) add('street', `Straße: ${this.street}`);
    return f;
  }

  // ponytail: every kind reads listPage, so paging one list refetches the other two small searches
  #page(scope: Scope) {
    // scope is read only past page 1, so switching scope does not refetch
    const offset =
      this.listPage > 1 && this.scope === scope
        ? (this.listPage - 1) * LIST_SIZE
        : 0;
    return { q: this.q, limit: LIST_SIZE, offset };
  }

  #lastMeetings?: Load<Hits<Meeting>>;
  #lastSpeeches?: Load<Hits<SpeechHit>>;
  #lastVotes?: Load<Hits<VoteHit>>;

  // untracked handles on the previous requests, read by the new Loads only
  /* eslint-disable ember/no-side-effects */
  @cached
  get meetings() {
    return (this.#lastMeetings = new Load(
      searchMeetings(this.#page('sitzungen')),
      this.#lastMeetings,
    ));
  }

  @cached
  get speeches() {
    return (this.#lastSpeeches = new Load(
      searchSpeeches(this.#page('reden')),
      this.#lastSpeeches,
    ));
  }

  @cached
  get votes() {
    return (this.#lastVotes = new Load(
      searchVotes(this.#page('abstimmungen')),
      this.#lastVotes,
    ));
  }
  /* eslint-enable ember/no-side-effects */

  people = new Load<PersonHit[]>(getPeople());
  committees = new Load(getCommitteeList());

  @cached
  get personIds() {
    return new Map((this.people.value ?? []).map((p) => [p.name, p.person_id]));
  }

  @cached
  get personHits() {
    const q = this.q.trim();
    const all = this.people.value ?? [];
    if (!q) return [...all].sort((a, b) => a.name.localeCompare(b.name, 'de'));
    return all
      .filter((p) => matches(q, [p.name, p.party, p.party_code]))
      .sort(
        (a, b) =>
          rankBy(q, a.name) - rankBy(q, b.name) ||
          a.name.localeCompare(b.name, 'de'),
      );
  }

  @cached
  get committeeHits() {
    const q = this.q.trim();
    const all = this.committees.value ?? [];
    return all
      .filter((c) => !q || matches(q, [c.name]))
      .sort(
        (a, b) =>
          (q ? rankBy(q, a.name) - rankBy(q, b.name) : 0) ||
          b.meetings - a.meetings,
      );
  }

  // undefined while a kind is still loading or failed
  get counts(): Partial<Record<Scope, number>> {
    return {
      dokumente: this.search.value?.total,
      sitzungen: this.meetings.value?.total,
      personen: this.people.value ? this.personHits.length : undefined,
      gremien: this.committees.value ? this.committeeHits.length : undefined,
      reden: this.speeches.value?.total,
      abstimmungen: this.votes.value?.total,
    };
  }
}
