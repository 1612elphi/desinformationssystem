import Component from '@glimmer/component';
import { on } from '@ember/modifier';
import Icon from 'ui/components/icon';
import {
  bodyKind,
  isLive,
  startLabel,
  type CalMeeting,
} from 'ui/utils/calendar';
import { num } from 'ui/utils/meta';
import './calendar-chip.css';

// The kind colour already says Ortschaftsrat or Ausschuss.
const shortName = (name?: string) =>
  (name ?? '—').replace(/^(Ortschaftsrat|Ausschuss für)\s+/, '');

interface Signature {
  Element: HTMLButtonElement;
  Args: {
    meeting: CalMeeting;
    onOpen: (m: CalMeeting) => void;
    // full body name instead of the short one
    full?: boolean;
  };
}

export default class CalendarChip extends Component<Signature> {
  get m() {
    return this.args.meeting;
  }

  get kind() {
    return bodyKind(this.m.body_name);
  }

  get live() {
    return isLive(this.m);
  }

  get name() {
    return this.args.full
      ? (this.m.body_name ?? '—')
      : shortName(this.m.body_name);
  }

  get label() {
    return [
      this.m.time,
      this.m.body_name,
      this.live ? 'live' : '',
      this.votes ? `${num(this.votes)} Abstimmungen` : '',
      this.docs ? `${num(this.docs)} Dokumente` : '',
    ]
      .filter(Boolean)
      .join(' · ');
  }

  get start() {
    return startLabel(this.m.time);
  }

  get votes() {
    return this.m.vote_count ?? 0;
  }

  get docs() {
    return this.m.documents ?? 0;
  }

  open = () => this.args.onOpen(this.m);

  <template>
    <button
      type="button"
      class="dis-cal-chip"
      data-kind={{this.kind}}
      data-live={{if this.live "true"}}
      title={{this.label}}
      aria-label={{this.label}}
      {{on "click" this.open}}
      ...attributes
    >
      {{#if this.live}}<span
          class="dis-cal-pulse"
          aria-hidden="true"
        ></span>{{/if}}
      {{#if this.start}}
        <span class="dis-cal-chip-time">{{this.start}}</span>
      {{/if}}
      <span class="dis-cal-chip-name">{{this.name}}</span>
      {{#if this.votes}}
        <span class="dis-cal-mark" aria-hidden="true">
          <Icon @name="checkmark" @size={{11}} />
        </span>
      {{/if}}
      {{#if this.docs}}
        <span class="dis-cal-mark" aria-hidden="true">
          <Icon @name="document" @size={{11}} />
        </span>
      {{/if}}
    </button>
  </template>
}
