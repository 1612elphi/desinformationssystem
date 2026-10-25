import Component from '@glimmer/component';
import { fn, hash } from '@ember/helper';
import { htmlSafe } from '@ember/template';
import { modifier } from 'ember-modifier';
import DocModal from 'ui/components/doc-modal';
import MeetingModal from 'ui/components/meeting-modal';
import PersonModal from 'ui/components/person-modal';
import VorlageModal from 'ui/components/vorlage-modal';
import type { Pane } from 'ui/services/nav';
import './pane-stack.css';

interface Signature {
  Args: {
    stack: Pane[];
    onPopTo: (index: number) => void;
  };
}

const eq = (a: unknown, b: unknown) => a === b;
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
  get open() {
    return this.args.stack.length > 0;
  }

  // a string key per pane, so a pane keeps its component and data as others push and pop
  get items() {
    return this.args.stack.map((p) => ({ ...p, key: `${p.kind}:${p.id}` }));
  }

  // spines plus the top pane; a dialog sizes to its content otherwise, which squeezes the pane
  get width() {
    const top = this.args.stack.at(-1);
    const pane = top?.kind === 'meeting' ? 1120 : 720;
    return htmlSafe(
      `--dis-panes-width: ${(this.args.stack.length - 1) * 40 + pane}px`,
    );
  }

  isTop = (i: number) => i === this.args.stack.length - 1;
  closeAt = (i: number) => this.args.onPopTo(i - 1);
  popTop = () => this.args.onPopTo(this.args.stack.length - 2);
  closeAll = () => this.args.onPopTo(-1);

  <template>
    <dialog
      class="dis-panes"
      style={{this.width}}
      aria-label="Ansicht"
      {{stackDialog this.open this.popTop this.closeAll}}
    >
      {{#each this.items key="key" as |p i|}}
        {{#let
          (hash
            collapsed=(if (this.isTop i) false true)
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
