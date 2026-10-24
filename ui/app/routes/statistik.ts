import Route from '@ember/routing/route';
import { getAnalytics, type Analytics } from 'ui/api';
import Load from 'ui/utils/load';

export default class StatistikRoute extends Route {
  #analytics?: Load<Analytics>;

  // Fetched on the first visit, then kept for the session.
  model() {
    return (this.#analytics ??= new Load(getAnalytics()));
  }
}
