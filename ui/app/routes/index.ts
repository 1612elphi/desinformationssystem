import Route from '@ember/routing/route';
import type { ApplicationModel } from 'ui/routes/application';

const replace = { replace: true };

export default class IndexRoute extends Route {
  queryParams = {
    q: replace,
    committee: replace,
    type: replace,
    cat: replace,
    sub: replace,
    pub: replace,
    from: replace,
    to: replace,
    topic: replace,
    district: replace,
    street: replace,
    page: replace,
  };

  model() {
    return this.modelFor('application') as ApplicationModel;
  }
}
