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

// "meeting:termin-10574,person:65290"; ids contain no comma or colon
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

const NO_PANES = { stack: '', doc: '', meeting: '', person: '', vorlage: '' };

// Open panes and filters live in the query string, so every link is shareable.
export default class NavService extends Service {
  @service declare router: RouterService;

  // panes pushed in this tab, so closing can step back through history
  private pushed = 0;

  // index of the pane the last click or focus was in; set by the pane stack
  origin?: number;

  get stack(): Pane[] {
    const app = getOwner(this)?.lookup('controller:application') as
      ApplicationController | undefined;
    return parseStack(app?.stack ?? '');
  }

  // push a pane; one already in the stack comes back to the top instead
  open(modals: Modals) {
    const kind = KINDS.find((k) => modals[k]);
    if (!kind) return;
    const pane = { kind, id: modals[kind]! };
    let stack = this.stack;
    const at = stack.findIndex((p) => p.kind === pane.kind && p.id === pane.id);
    if (at >= 0) return this.popTo(at);
    // opened from an earlier visible pane: it replaces the panes to that pane's right
    if (this.origin !== undefined && this.origin < stack.length - 1) {
      stack = stack.slice(0, this.origin + 1);
      // history no longer mirrors the stack depth, so pops replace from here on
      this.pushed = 0;
    } else {
      this.pushed++;
    }
    this.router.transitionTo({
      queryParams: { stack: formatStack([...stack, pane]) },
    });
  }

  // stepping inside a pane (next document, next meeting) keeps the depth
  replaceTop(kind: Overlay, id: string) {
    const stack = this.stack.slice(0, -1);
    this.router.replaceWith({
      queryParams: { stack: formatStack([...stack, { kind, id }]) },
    });
  }

  close() {
    this.popTo(this.stack.length - 2);
  }

  closeAll() {
    this.popTo(-1);
  }

  // keep panes 0..index
  popTo(index: number) {
    const drop = this.stack.length - 1 - index;
    if (drop <= 0) return;
    if (this.pushed >= drop) {
      this.pushed -= drop;
      history.go(-drop);
    } else {
      this.pushed = 0;
      this.router.replaceWith({
        queryParams: { stack: formatStack(this.stack.slice(0, index + 1)) },
      });
    }
  }

  // leave every pane for another view
  go(route: string, queryParams: Record<string, unknown> = {}) {
    this.pushed = 0;
    this.router.transitionTo(route, {
      queryParams: { ...NO_PANES, ...queryParams },
    });
  }

  filter(queryParams: Filters) {
    this.pushed = 0;
    this.router.transitionTo('index', {
      queryParams: { ...NO_PANES, page: 1, ...queryParams },
    });
  }
}

declare module '@ember/service' {
  interface Registry {
    nav: NavService;
  }
}
