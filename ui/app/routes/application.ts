import Route from '@ember/routing/route';
import { service } from '@ember/service';
import type RouterService from '@ember/routing/router-service';
import type Transition from '@ember/routing/transition';
import { getFacets, getStats } from 'ui/api';
import Load from 'ui/utils/load';
import { formatStack, parseStack, type Overlay } from 'ui/services/nav';

// Links from the React UI select the tab with ?tab=<slug> and one overlay
// with ?doc= and the like; both become the current URL shape.
const LEGACY_TABS = ['sitzungen', 'gremien', 'statistik', 'karte'];

export default class ApplicationRoute extends Route {
  @service declare router: RouterService;

  beforeModel(transition: Transition) {
    // application query params are not on transition.to; old links only arrive on page load
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
    if (!panes.length && !(tab && LEGACY_TABS.includes(tab))) return;
    // after the boot transition: redirecting inside it fails in the router
    // (_queryParamsFor reads an undefined route)
    q.delete('tab');
    for (const k of Object.keys(legacy)) q.delete(k);
    q.set('stack', formatStack([...parseStack(stack), ...panes]));
    if (!panes.length && !stack) q.delete('stack');
    const path =
      tab && LEGACY_TABS.includes(tab) ? `/${tab}` : location.pathname;
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
