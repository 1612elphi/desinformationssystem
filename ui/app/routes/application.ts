import Route from '@ember/routing/route';
import { service } from '@ember/service';
import type RouterService from '@ember/routing/router-service';
import type Transition from '@ember/routing/transition';
import { getFacets, getStats } from 'ui/api';
import Load from 'ui/utils/load';
import { formatStack, parseStack, type Overlay } from 'ui/services/nav';

// legacy react ui links
const SEARCH_PARAMS = [
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
  'cat',
  'page',
];

const LEGACY_TABS = ['sitzungen', 'gremien', 'statistik', 'reden', 'karte'];

export default class ApplicationRoute extends Route {
  @service declare router: RouterService;

  beforeModel(transition: Transition) {
    // old links: page load only
    if (transition.from) return;
    const q = new URLSearchParams(window.location.search);
    const get = (k: string) => q.get(k) ?? '';
    const tab = get('tab');
    const stack = get('stack');
    const legacy = {
      meeting: get('meeting'),
      doc: get('doc'),
      person: get('person'),
      vorlage: get('vorlage'),
    };
    const panes = (Object.keys(legacy) as Overlay[])
      .filter((k) => legacy[k])
      .map((kind) => ({ kind, id: legacy[kind] }));
    // legacy search at /
    const oldSearch =
      location.pathname === '/' && SEARCH_PARAMS.some((k) => q.has(k));
    if (!panes.length && !(tab && LEGACY_TABS.includes(tab)) && !oldSearch)
      return;
    // router fails mid-boot redirect
    q.delete('tab');
    for (const k of Object.keys(legacy)) q.delete(k);
    q.set('stack', formatStack([...parseStack(stack), ...panes]));
    if (!panes.length && !stack) q.delete('stack');
    const path =
      tab && LEGACY_TABS.includes(tab)
        ? `/${tab}`
        : oldSearch
          ? '/suche'
          : location.pathname;
    const query = q.toString();
    setTimeout(() =>
      this.router.replaceWith(query ? `${path}?${query}` : path),
    );
  }

  model() {
    return {
      facets: new Load(getFacets()),
      stats: new Load(getStats()),
    };
  }
}

export type ApplicationModel = ReturnType<ApplicationRoute['model']>;
