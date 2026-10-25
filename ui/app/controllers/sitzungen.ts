import Controller from '@ember/controller';
import { tracked, cached } from '@glimmer/tracking';
import { getMeetings } from 'ui/api';
import Load from 'ui/utils/load';

export const MEETINGS_PAGE = 100;

export default class SitzungenController extends Controller {
  queryParams = ['upcoming', 'committee', 'page'];

  @tracked upcoming = true;
  @tracked committee = '';
  @tracked page = 1;

  setUpcoming = (v: boolean) => {
    this.upcoming = v;
    this.page = 1;
  };

  setCommittee = (v: string) => {
    this.committee = v;
    this.page = 1;
  };

  setPage = (p: number) => (this.page = p);

  @cached
  get meetings() {
    return new Load(
      getMeetings({
        upcoming: this.upcoming,
        committee: this.committee,
        limit: MEETINGS_PAGE,
        offset: (this.page - 1) * MEETINGS_PAGE,
      }),
    );
  }

  get icsUrl() {
    return `/api/meetings.ics${this.committee ? `?committee=${encodeURIComponent(this.committee)}` : ''}`;
  }
}
