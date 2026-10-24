import Route from '@ember/routing/route';
import { getAnalytics, type Analytics } from 'ui/api';
import Load from 'ui/utils/load';

export default class StatistikRoute extends Route {
  #analytics?: Load<Analytics>;

  // kept for the session once it succeeds; a failed load is retried on the next visit
  model() {
    if (!this.#analytics || this.#analytics.error)
      this.#analytics = new Load(getAnalytics());
    return this.#analytics;
  }
}
