import Service, { service } from '@ember/service';
import type RouterService from '@ember/routing/router-service';

type Modals = Partial<Record<'doc' | 'meeting' | 'person' | 'vorlage', string>>;
type Filters = Partial<Record<'topic' | 'district' | 'street', string>>;

// Open modals and filters live in the query string, so every link is shareable.
export default class NavService extends Service {
  @service declare router: RouterService;

  open(queryParams: Modals) {
    this.router.replaceWith({ queryParams });
  }

  filter(queryParams: Filters) {
    this.router.transitionTo('index', {
      queryParams: { doc: '', ...queryParams },
    });
  }
}

declare module '@ember/service' {
  interface Registry {
    nav: NavService;
  }
}
