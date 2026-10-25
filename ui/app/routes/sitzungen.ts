import Route from '@ember/routing/route';
import type { ApplicationModel } from 'ui/routes/application';

const replace = { replace: true };

export default class SitzungenRoute extends Route {
  queryParams = {
    view: replace,
    day: replace,
    kind: replace,
    upcoming: replace,
    committee: replace,
    page: replace,
  };

  model() {
    return this.modelFor('application') as ApplicationModel;
  }
}
