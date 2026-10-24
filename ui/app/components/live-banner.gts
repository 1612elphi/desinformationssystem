import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { on } from '@ember/modifier';
import { registerDestructor } from '@ember/destroyable';
import type Owner from '@ember/owner';
import { getLive, type Meeting } from 'ui/api';
import Icon from 'ui/components/icon';
import type NavService from 'ui/services/nav';

// Polls /api/live for a session happening right now.
export default class LiveBanner extends Component {
  @service declare nav: NavService;
  @tracked live: Meeting[] = [];

  constructor(owner: Owner, args: object) {
    super(owner, args);
    const tick = () =>
      getLive()
        .then((r) => {
          if (!this.isDestroying) this.live = r.meetings || [];
        })
        .catch(() => {});
    void tick();
    const iv = setInterval(tick, 60_000);
    registerDestructor(this, () => clearInterval(iv));
  }

  get text() {
    const first = this.live[0];
    if (!first) return '';
    const what =
      this.live.length === 1
        ? `${first.body_name || first.title || 'Sitzung'} tagt gerade`
        : `${this.live.length} Sitzungen tagen gerade`;
    return what + (first.time ? ` · ${first.time}` : '');
  }

  openFirst = () => {
    const first = this.live[0];
    if (first) this.nav.open({ meeting: first.id });
  };

  <template>
    {{#if this.live.length}}
      <div class="dis-live" role="status">
        <span class="dis-live-pulse" aria-hidden="true"></span>
        <span class="dis-live-label">Jetzt live</span>
        <span class="dis-live-text">{{this.text}}</span>
        <button
          type="button"
          class="cds--btn cds--btn--primary cds--btn--sm"
          {{on "click" this.openFirst}}
        >
          Protokoll ansehen
          <Icon @name="time" class="cds--btn__icon" />
        </button>
      </div>
    {{/if}}
  </template>
}
