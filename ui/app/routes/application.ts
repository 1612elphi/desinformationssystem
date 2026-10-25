import Route from '@ember/routing/route';
import { service } from '@ember/service';
import type RouterService from '@ember/routing/router-service';
import type Transition from '@ember/routing/transition';
import { getFacets, getStats } from 'ui/api';
import Load from 'ui/utils/load';

// Links from the React UI select the tab with ?tab=<slug>.
const LEGACY_TABS = ['sitzungen', 'gremien', 'statistik', 'karte'];

export default class ApplicationRoute extends Route {
  @service declare router: RouterService;

  beforeModel(transition: Transition) {
    const { tab, doc, meeting, person, vorlage } = (transition.to
      ?.queryParams ?? {}) as Record<string, string | undefined>;
    if (tab && LEGACY_TABS.includes(tab)) {
      const queryParams = Object.fromEntries(
        Object.entries({ doc, meeting, person, vorlage }).filter(([, v]) => v),
      );
      this.router.replaceWith(tab, { queryParams });
    }
  }

  model() {
    return {
      facets: new Load(getFacets()),
      stats: new Load(getStats()),
    };
  }
}

export type ApplicationModel = ReturnType<ApplicationRoute['model']>;
