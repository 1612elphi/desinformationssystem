import Controller from '@ember/controller';
import { cached, tracked } from '@glimmer/tracking';
import { getSpeechAnalytics, type SpeechAnalytics } from 'ui/utils/api-reden';
import Load from 'ui/utils/load';

export default class RedenController extends Controller {
  // ?meeting= opens legacy pane
  queryParams = [{ meeting: 'sitzung' }];

  @tracked meeting = '';

  #last?: Load<SpeechAnalytics>;

  // picker needs every meeting
  @cached
  get all() {
    return new Load(getSpeechAnalytics());
  }

  // keep stale until loaded
  @cached
  get load() {
    const meeting = this.meeting;
    // untracked handle on the previous request, read by the new Load only
    // eslint-disable-next-line ember/no-side-effects
    return (this.#last = meeting
      ? new Load(getSpeechAnalytics(meeting), this.#last)
      : this.all);
  }

  get meetings() {
    return this.all.value?.meetings ?? [];
  }

  setMeeting = (id: string) => (this.meeting = id);
}
