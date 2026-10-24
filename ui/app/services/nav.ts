import Service, { service } from '@ember/service';
import type RouterService from '@ember/routing/router-service';

export type Overlay = 'doc' | 'meeting' | 'person' | 'vorlage';
type Modals = Partial<Record<Overlay, string>>;
type Filters = Partial<Record<'topic' | 'district' | 'street', string>>;

// Open overlays and filters live in the query string, so every link is shareable.
export default class NavService extends Service {
  @service declare router: RouterService;

  // overlays opened in this tab, so close can step back instead of pushing
  private pushed = 0;

  open(queryParams: Modals) {
    this.pushed++;
    this.router.transitionTo({ queryParams });
  }

  close(key: Overlay) {
    if (this.pushed > 0) {
      this.pushed--;
      history.back();
    } else {
      this.router.replaceWith({ queryParams: { [key]: '' } });
    }
  }

  filter(queryParams: Filters) {
    this.pushed = 0;
    this.router.transitionTo('index', {
      queryParams: {
        doc: '',
        meeting: '',
        person: '',
        vorlage: '',
        ...queryParams,
      },
    });
  }
}

declare module '@ember/service' {
  interface Registry {
    nav: NavService;
  }
}
