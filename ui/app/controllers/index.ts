import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import { search, type SearchResult } from 'ui/api';
import type { Column } from 'ui/components/column-picker';
import Load from 'ui/utils/load';
import { PUBLIC_OPTS, SUBMITTERS } from 'ui/utils/meta';

export const PAGE_SIZE = 25;

export const ALL_COLUMNS: Column[] = [
  { key: 'date', label: 'Datum', cls: 'dis-c-date' },
  { key: 'gremium', label: 'Gremium', cls: 'dis-c-gremium' },
  { key: 'top', label: 'TOP', cls: 'dis-c-top' },
  { key: 'sub', label: 'Einbringer', cls: 'dis-c-sub' },
  { key: 'typ', label: 'Typ', cls: 'dis-c-typ' },
  { key: 'doc', label: 'Dokument', cls: 'dis-c-doc' },
  { key: 'cat', label: 'Kategorie', cls: 'dis-c-cat' },
  { key: 'stadtteil', label: 'Stadtteil', cls: 'dis-c-stadtteil' },
];
const COL_KEYS = ALL_COLUMNS.map((c) => c.key);
const COLS_STORAGE = 'dis-cols';

function storedCols(): string[] {
  try {
    const s: unknown = JSON.parse(localStorage.getItem(COLS_STORAGE) || 'null');
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
    'sub',
    'pub',
    'from',
    'to',
    'topic',
    'district',
    'street',
  ];

  @tracked q = '';
  @tracked committee = '';
  @tracked type = '';
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
    try {
      localStorage.setItem(COLS_STORAGE, JSON.stringify(keys));
    } catch {
      // storage blocked: the choice lasts for this page view only
    }
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
    }, 350);
  };

  clearQ = () => {
    clearTimeout(this.#qTimer);
    this.qDraft = null;
    this.q = '';
  };

  setFilter = (key: FilterKey, value: string) => {
    this[key] = value;
  };

  resetAll = () => {
    this.clearQ();
    this.committee = '';
    this.type = '';
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
      public: this.pub,
      from: this.from,
      to: this.to,
      topic: this.topic,
      submitter: this.sub,
      district: this.district,
      street: this.street,
    };
  }

  get filterKey() {
    return JSON.stringify(this.params);
  }

  // Changing any filter starts again at page 1.
  @tracked pageOf = { key: '', page: 1 };

  get page() {
    return this.pageOf.key === this.filterKey ? this.pageOf.page : 1;
  }

  setPage = (page: number) => {
    this.pageOf = { key: this.filterKey, page };
  };

  #lastSearch?: Load<SearchResult>;

  @cached
  get search() {
    const req = search({
      ...this.params,
      limit: PAGE_SIZE,
      offset: (this.page - 1) * PAGE_SIZE,
    });
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#lastSearch = new Load(req, this.#lastSearch));
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
