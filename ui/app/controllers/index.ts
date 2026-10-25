import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import { searchFaceted, type FacetedResult } from 'ui/utils/api-dokumente';
import type { Column } from 'ui/components/data-table';
import Load from 'ui/utils/load';
import { PUBLIC_OPTS, SUBMITTERS } from 'ui/utils/meta';

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

export default class IndexController extends Controller {
  queryParams = [
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
      this.cursor = -1;
    }, 350);
  };

  clearQ = () => {
    clearTimeout(this.#qTimer);
    this.qDraft = null;
    this.q = '';
    this.page = 1;
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
    if (this.sub)
      add('sub', `Einbringer: ${SUBMITTERS[this.sub]?.name || this.sub}`);
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
}
