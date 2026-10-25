import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import { uniqueId } from '@ember/helper';
import CanonButton from 'delphicanon/components/canon-button';
import { modifier } from 'ember-modifier';
import Icon from 'ui/components/icon';
import './modal.css';

export interface PaneContext {
  collapsed: boolean;
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
const KIND_ICON: Record<string, string> = {
  doc: 'document',
  meeting: 'calendar',
  person: 'user',
  vorlage: 'file-stack',
};
const kindIcon = (kind?: string) => (kind ? (KIND_ICON[kind] ?? '') : '');

const focusWhen = modifier((el: HTMLElement, [active]: [boolean?]) => {
  if (active) el.focus({ preventScroll: true });
});

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
      <span class="dis-pane-spine-kind"><Icon
          @name={{kindIcon @pane.kind}}
          @size={{14}}
        />{{kindLabel @pane.kind}}</span>
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
          <span class="dis-pane-kind"><Icon
              @name={{kindIcon @pane.kind}}
              @size={{12}}
            />{{kindLabel @pane.kind}}</span>
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
