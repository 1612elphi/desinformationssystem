import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import { uniqueId } from '@ember/helper';
import CanonButton from 'delphicanon/components/canon-button';
import { modifier } from 'ember-modifier';
import Icon from 'ui/components/icon';
import './modal.css';

export interface PaneContext {
  // below the visible panes: drawn as a spine
  collapsed: boolean;
  // the newest pane: it holds focus and the keyboard shortcuts
  active: boolean;
  index: number;
  kind: string;
  onSelect: () => void;
}

const KIND_LABEL: Record<string, string> = {
  doc: 'Dokument',
  meeting: 'Sitzung',
  person: 'Person',
  vorlage: 'Vorlage',
};
const kindLabel = (kind?: string) => (kind ? (KIND_LABEL[kind] ?? kind) : '');

// the newest pane takes focus, so the keyboard follows the stack
const focusWhen = modifier((el: HTMLElement, [active]: [boolean?]) => {
  if (active) el.focus({ preventScroll: true });
});

// One pane in the stack: the full view on top, a spine with its title below.
const Modal: TOC<{
  Args: {
    heading: string;
    onClose: () => void;
    pane?: PaneContext;
  };
  Blocks: { default: [] };
}> = <template>
  {{#if @pane.collapsed}}
    <button
      type="button"
      class="dis-pane-spine"
      title={{@heading}}
      {{on "click" @pane.onSelect}}
    >
      <span class="dis-pane-spine-kind">{{kindLabel @pane.kind}}</span>
      <span class="dis-pane-spine-title">{{@heading}}</span>
    </button>
  {{else}}
    {{#let (uniqueId) as |hid|}}
      <section
        class="dis-pane {{if @pane.active 'dis-pane--active' 'dis-pane--side'}}"
        data-pane-index={{@pane.index}}
        aria-labelledby={{hid}}
      >
        <header class="dis-pane-head">
          <span class="dis-pane-kind">{{kindLabel @pane.kind}}</span>
          <h2
            id={{hid}}
            class="dis-pane-title"
            tabindex="-1"
            {{focusWhen @pane.active}}
          >
            {{@heading}}
          </h2>
          <CanonButton
            class="dis-pane-close"
            @kind="cell"
            @icon={{component Icon name="close"}}
            @label="Schließen"
            @onClick={{@onClose}}
          />
        </header>
        <div class="dis-pane-body">{{yield}}</div>
      </section>
    {{/let}}
  {{/if}}
</template>;

export default Modal;
