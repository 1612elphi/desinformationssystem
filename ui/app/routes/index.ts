import Route from '@ember/routing/route';
import type { ApplicationModel } from 'ui/routes/application';
import { getLastSession, getNextSessions } from 'ui/utils/api-uebersicht';
import Load from 'ui/utils/load';

export default class IndexRoute extends Route {
  model() {
    const today = new Date();
    return {
      app: this.modelFor('application') as ApplicationModel,
      last: new Load(getLastSession()),
      next: new Load(getNextSessions(today)),
    };
  }
}

export type IndexModel = ReturnType<IndexRoute['model']>;
