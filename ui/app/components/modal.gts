import type { TOC } from '@ember/component/template-only';
import CanonButton from 'delphicanon/components/canon-button';
import CanonSheet from 'delphicanon/components/canon-sheet';
import Icon from 'ui/components/icon';
import './modal.css';

// Side sheet for one detail view; the dialog traps focus and returns it on close.
const Modal: TOC<{
  Args: {
    heading: string;
    open: boolean;
    onClose: () => void;
    wide?: boolean;
  };
  Blocks: { default: [] };
}> = <template>
  <CanonSheet
    class="dis-sheet {{if @wide 'dis-sheet--wide'}}"
    @open={{@open}}
    @edge="end"
    @title={{@heading}}
    @onClose={{@onClose}}
  >
    <:header as |close|>
      <CanonButton
        class="dis-sheet-close"
        @kind="cell"
        @icon={{component Icon name="close"}}
        @label="Schließen"
        @onClick={{close}}
      />
    </:header>
    <:default>
      <div class="dis-sheet-body">{{yield}}</div>
    </:default>
  </CanonSheet>
</template>;

export default Modal;
