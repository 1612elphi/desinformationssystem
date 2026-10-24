import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import Icon from 'ui/components/icon';
import { dialog } from 'ui/utils/modifiers';

const onBackdrop = (close: () => void) => (e: MouseEvent) => {
  if (e.target === e.currentTarget) close();
};

// Carbon passive modal, size lg.
const Modal: TOC<{
  Args: { heading: string; onClose: () => void };
  Blocks: { default: [] };
}> = <template>
  <div
    class="cds--modal is-visible"
    role="presentation"
    {{on "click" (onBackdrop @onClose)}}
  >
    <div
      class="cds--modal-container cds--modal-container--lg"
      role="dialog"
      aria-modal="true"
      aria-label={{@heading}}
      tabindex="-1"
      {{dialog @onClose}}
    >
      <div class="cds--modal-header">
        <h2 class="cds--modal-header__heading">{{@heading}}</h2>
      </div>
      <div class="cds--modal-close-button">
        <button
          type="button"
          class="cds--modal-close"
          aria-label="Close"
          {{on "click" @onClose}}
        >
          <Icon @name="close" @size={{20}} class="cds--modal-close__icon" />
        </button>
      </div>
      <div class="cds--modal-content">
        {{yield}}
      </div>
    </div>
  </div>
</template>;

export default Modal;
