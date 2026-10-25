import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { service } from '@ember/service';
import { registerDestructor } from '@ember/destroyable';
import type Owner from '@ember/owner';
import { fn, hash } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import { modifier } from 'ember-modifier';
import DocModal from 'ui/components/doc-modal';
import MeetingModal from 'ui/components/meeting-modal';
import PersonModal from 'ui/components/person-modal';
import VorlageModal from 'ui/components/vorlage-modal';
import type NavService from 'ui/services/nav';
import type { Pane } from 'ui/services/nav';
import { eq } from 'ui/utils/meta';
import './pane-stack.css';

interface Signature {
  Args: {
    stack: Pane[];
    onPopTo: (index: number) => void;
  };
}

// pane widths in px: the newest at its own width, earlier ones beside it narrower
const SPINE = 40;
const SIDE = 520;
const own = (p?: Pane) => (p?.kind === 'meeting' ? 1120 : 720);
// element that had focus before the stack opened, to return it on close
const openers = new WeakMap<HTMLDialogElement, Element | null>();

// Open while the stack has panes: the browser traps focus and makes the page
// inert; Escape pops one pane, a click on the backdrop closes them all.
const stackDialog = modifier(
  (
    el: HTMLDialogElement,
    [open, popTop, closeAll]: [boolean, () => void, () => void],
  ) => {
    if (open && !el.open) {
      openers.set(el, document.activeElement);
      el.showModal();
    } else if (!open && el.open) {
      el.close();
      const opener = openers.get(el);
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
    }
    const onCancel = (e: Event) => {
      e.preventDefault();
      popTop();
    };
    const onClick = (e: MouseEvent) => {
      if (e.target === el) closeAll();
    };
    el.addEventListener('cancel', onCancel);
    el.addEventListener('click', onClick);
    return () => {
      el.removeEventListener('cancel', onCancel);
      el.removeEventListener('click', onClick);
    };
  },
);

export default class PaneStack extends Component<Signature> {
  @service declare nav: NavService;
  @tracked viewport = window.innerWidth;

  constructor(owner: Owner, args: Signature['Args']) {
    super(owner, args);
    const onResize = () => (this.viewport = window.innerWidth);
    window.addEventListener('resize', onResize);
    registerDestructor(this, () =>
      window.removeEventListener('resize', onResize),
    );
  }

  get open() {
    return this.args.stack.length > 0;
  }

  // a string key per pane, so a pane keeps its component and data as others push and pop
  get items() {
    return this.args.stack.map((p) => ({ ...p, key: `${p.kind}:${p.id}` }));
  }

  get room() {
    return this.viewport < 640 ? this.viewport : this.viewport - 24;
  }

  // index of the first full pane: as many as fit next to the newest, spines before
  get firstFull() {
    const n = this.args.stack.length;
    let first = n - 1;
    const width = (f: number) =>
      own(this.args.stack[n - 1]) + (n - 1 - f) * SIDE + f * SPINE;
    while (first > 0 && width(first - 1) <= this.room) first--;
    return first;
  }

  get width() {
    const n = this.args.stack.length;
    const f = this.firstFull;
    const w = own(this.args.stack[n - 1]) + (n - 1 - f) * SIDE + f * SPINE;
    return htmlSafe(`--dis-panes-width: ${w}px`);
  }

  isTop = (i: number) => i === this.args.stack.length - 1;
  isCollapsed = (i: number) => i < this.firstFull;
  closeAt = (i: number) => this.args.onPopTo(i - 1);
  popTop = () => this.args.onPopTo(this.args.stack.length - 2);
  closeAll = () => this.args.onPopTo(-1);

  // the pane a click or focus came from; opening from it replaces the panes to its right
  noteOrigin = (e: Event) => {
    const el = (e.target as Element | null)?.closest('[data-pane-index]');
    this.nav.origin = el
      ? Number(el.getAttribute('data-pane-index'))
      : undefined;
  };

  <template>
    <dialog
      class="dis-panes"
      style={{this.width}}
      aria-label="Ansicht"
      {{stackDialog this.open this.popTop this.closeAll}}
      {{on "click" this.noteOrigin capture=true}}
      {{on "focusin" this.noteOrigin}}
    >
      {{#each this.items key="key" as |p i|}}
        {{#let
          (hash
            collapsed=(this.isCollapsed i)
            active=(this.isTop i)
            index=i
            kind=p.kind
            onSelect=(fn @onPopTo i)
          )
          as |pane|
        }}
          {{#if (eq p.kind "doc")}}
            <DocModal
              @id={{p.id}}
              @pane={{pane}}
              @onClose={{fn this.closeAt i}}
            />
          {{else if (eq p.kind "meeting")}}
            <MeetingModal
              @id={{p.id}}
              @pane={{pane}}
              @onClose={{fn this.closeAt i}}
            />
          {{else if (eq p.kind "person")}}
            <PersonModal
              @id={{p.id}}
              @pane={{pane}}
              @onClose={{fn this.closeAt i}}
            />
          {{else if (eq p.kind "vorlage")}}
            <VorlageModal
              @nr={{p.id}}
              @pane={{pane}}
              @onClose={{fn this.closeAt i}}
            />
          {{/if}}
        {{/let}}
      {{/each}}
    </dialog>
  </template>
}
