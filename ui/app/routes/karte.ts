import Route from '@ember/routing/route';
import type { ApplicationModel } from 'ui/routes/application';

export default class KarteRoute extends Route {
  model() {
    return this.modelFor('application') as ApplicationModel;
  }
}
