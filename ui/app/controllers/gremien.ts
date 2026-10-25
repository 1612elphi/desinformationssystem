import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import { getCommittee, getCommittees } from 'ui/api';
import {
  GROUP_ORDER,
  byChamber,
  committeeGroup,
  type CommitteeDetail,
} from 'ui/utils/api-menschen';
import Load from 'ui/utils/load';

export default class GremienController extends Controller {
  queryParams = [{ selectedId: 'gremium' }];

  @tracked selectedId = '';
  @tracked query = '';

  select = (id: string) => (this.selectedId = id);

  // with no gremium in the URL the view opens on the Gemeinderat
  get currentId() {
    return (
      this.selectedId ||
      this.list.value?.find((c) => c.name === 'Gemeinderat')?.id ||
      ''
    );
  }
  setQuery = (q: string) => (this.query = q);

  @cached
  get list() {
    return new Load(getCommittees());
  }

  @cached
  get selected() {
    return this.currentId
      ? new Load<CommitteeDetail>(getCommittee(this.currentId))
      : undefined;
  }

  @cached
  get groups() {
    const q = this.query.trim().toLocaleLowerCase('de');
    const items = (this.list.value ?? []).filter((c) =>
      c.name.toLocaleLowerCase('de').includes(q),
    );
    return GROUP_ORDER.map((name) => ({
      name,
      items: items.filter((c) => committeeGroup(c) === name),
    })).filter((g) => g.items.length);
  }

  get visible() {
    return this.groups.flatMap((g) => g.items);
  }

  // Members of the selected body in seat order; the person sheet steps through them.
  @cached
  get members() {
    return byChamber(this.selected?.value?.members ?? []);
  }
}
