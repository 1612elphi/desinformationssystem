import Service, { service } from '@ember/service';
import { getOwner } from '@ember/owner';
import type RouterService from '@ember/routing/router-service';
import type ApplicationController from 'ui/controllers/application';

export type Overlay = 'doc' | 'meeting' | 'person' | 'vorlage';
type Modals = Partial<Record<Overlay, string>>;
type Filters = Partial<
  Record<'topic' | 'district' | 'street' | 'cat' | 'q', string>
>;

export interface Pane {
  kind: Overlay;
  id: string;
}

const KINDS: Overlay[] = ['doc', 'meeting', 'person', 'vorlage'];

// ids lack comma, colon
export function parseStack(s: string): Pane[] {
  return s
    .split(',')
    .map((part) => {
      const i = part.indexOf(':');
      return { kind: part.slice(0, i) as Overlay, id: part.slice(i + 1) };
    })
    .filter((p) => KINDS.includes(p.kind) && p.id);
}

export const formatStack = (panes: Pane[]) =>
  panes.map((p) => `${p.kind}:${p.id}`).join(',');

const NO_PANES = { stack: '' };

export default class NavService extends Service {
  @service declare router: RouterService;

  // lets close use history
  private pushed = 0;
  private expected = '';

  constructor(...args: ConstructorParameters<typeof Service>) {
    super(...args);
    // history no longer mirrors
    this.router.on('routeDidChange', () => {
      if (formatStack(this.stack) !== this.expected) this.pushed = 0;
    });
  }

  private to(stack: Pane[]) {
    this.expected = formatStack(stack);
    return { queryParams: { stack: this.expected } };
  }

  origin?: number;

  get stack(): Pane[] {
    const app = getOwner(this)?.lookup('controller:application') as
      ApplicationController | undefined;
    return parseStack(app?.stack ?? '');
  }

  open(modals: Modals) {
    const kind = KINDS.find((k) => modals[k]);
    if (!kind) return;
    const pane = { kind, id: modals[kind]! };
    let stack = this.stack;
    const at = stack.findIndex((p) => p.kind === pane.kind && p.id === pane.id);
    if (at >= 0) return this.popTo(at);
    // replaces panes to right
    if (this.origin !== undefined && this.origin < stack.length - 1) {
      stack = stack.slice(0, this.origin + 1);
      // history out of sync
      this.pushed = 0;
    } else {
      this.pushed++;
    }
    this.router.transitionTo(this.to([...stack, pane]));
  }

  replaceTop(kind: Overlay, id: string) {
    const stack = this.stack.slice(0, -1);
    this.router.replaceWith(this.to([...stack, { kind, id }]));
  }

  close() {
    this.popTo(this.stack.length - 2);
  }

  closeAll() {
    this.popTo(-1);
  }

  popTo(index: number) {
    const drop = this.stack.length - 1 - index;
    if (drop <= 0) return;
    if (this.pushed >= drop) {
      this.pushed -= drop;
      this.to(this.stack.slice(0, index + 1));
      history.go(-drop);
    } else {
      this.pushed = 0;
      this.router.replaceWith(this.to(this.stack.slice(0, index + 1)));
    }
  }

  go(route: string, queryParams: Record<string, unknown> = {}) {
    this.pushed = 0;
    this.router.transitionTo(route, {
      queryParams: { ...NO_PANES, ...queryParams },
    });
  }

  filter(queryParams: Filters) {
    this.pushed = 0;
    this.router.transitionTo('suche', {
      queryParams: { ...NO_PANES, page: 1, scope: 'dokumente', ...queryParams },
    });
  }
}

declare module '@ember/service' {
  interface Registry {
    nav: NavService;
  }
}
