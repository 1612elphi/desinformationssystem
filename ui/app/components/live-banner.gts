import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { registerDestructor } from '@ember/destroyable';
import type Owner from '@ember/owner';
import CanonButton from 'delphicanon/components/canon-button';
import { getLive, type Meeting } from 'ui/api';
import Icon from 'ui/components/icon';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import './live-banner.css';

// Polls /api/live for a session happening right now, while the tab is visible.
export default class LiveBanner extends Component {
  @service declare nav: NavService;
  @tracked live: Meeting[] = [];

  constructor(owner: Owner, args: object) {
    super(owner, args);
    const tick = () => {
      if (document.hidden) return;
      getLive()
        .then((r) => {
          if (!this.isDestroying) this.live = r.meetings || [];
        })
        .catch(() => {});
    };
    tick();
    const iv = setInterval(tick, 60_000);
    document.addEventListener('visibilitychange', tick);
    registerDestructor(this, () => {
      clearInterval(iv);
      document.removeEventListener('visibilitychange', tick);
    });
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
    <div role="status">
      {{#if this.live.length}}
        <div class="dis-live">
          <span class="dis-live-pulse" aria-hidden="true"></span>
          <span
            class="dis-live-label"
            {{rough "circle" color="--canon-destructive" padding=5}}
          ><Icon @name="radio-tower" @size={{14}} />
            Jetzt live</span>
          <span class="dis-live-text">{{this.text}}</span>
          <CanonButton
            class="dis-live-open"
            @kind="cell"
            @text="Protokoll ansehen"
            @icon={{component Icon name="time"}}
            @onClick={{this.openFirst}}
          />
        </div>
      {{/if}}
    </div>
  </template>
}
