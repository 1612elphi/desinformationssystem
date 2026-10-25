import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import { getCommittee, getCommittees } from 'ui/api';
import Load from 'ui/utils/load';

export default class GremienController extends Controller {
  queryParams = [{ selectedId: 'gremium' }];

  @tracked selectedId = '';

  select = (id: string) => (this.selectedId = id);

  @cached
  get list() {
    return new Load(getCommittees());
  }

  @cached
  get selected() {
    return this.selectedId
      ? new Load(getCommittee(this.selectedId))
      : undefined;
  }
}
