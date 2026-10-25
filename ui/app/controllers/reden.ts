import Controller from '@ember/controller';
import { cached, tracked } from '@glimmer/tracking';
import { getSpeechAnalytics, type SpeechAnalytics } from 'ui/utils/api-reden';
import Load from 'ui/utils/load';

export default class RedenController extends Controller {
  // ?meeting= is taken: the application reads it as an old link to the meeting pane
  queryParams = [{ meeting: 'sitzung' }];

  @tracked meeting = '';

  #last?: Load<SpeechAnalytics>;

  // unfiltered: the meeting picker needs every meeting, a filtered answer lists only its own
  @cached
  get all() {
    return new Load(getSpeechAnalytics());
  }

  // the previous meeting's numbers stay on screen until the next arrive
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
