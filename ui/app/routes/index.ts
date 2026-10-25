import Route from '@ember/routing/route';
import type IndexController from 'ui/controllers/index';
import type { ApplicationModel } from 'ui/routes/application';

const replace = { replace: true };

export default class IndexRoute extends Route {
  queryParams = {
    q: replace,
    committee: replace,
    type: replace,
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

  // runs after the query params are on the controller: the page from the URL holds
  setupController(controller: IndexController, model: ApplicationModel) {
    super.setupController(controller, model);
    controller.pageKey = controller.filterKey;
  }
}
