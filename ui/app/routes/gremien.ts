import Route from '@ember/routing/route';

export default class GremienRoute extends Route {
  queryParams = { selectedId: { replace: true } };
}
