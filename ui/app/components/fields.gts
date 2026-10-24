import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import Icon from 'ui/components/icon';
import { eq, inputValue } from 'ui/utils/meta';

export interface Item {
  id: string;
  label: string;
}

// Carbon Select (native <select>) in place of the React Dropdown.
export const SelectField: TOC<{
  Args: {
    id: string;
    label: string;
    items: Item[];
    value: string;
    onChange: (v: string) => void;
  };
}> = <template>
  <div class="cds--form-item">
    <div class="cds--select">
      <label for={{@id}} class="cds--label">{{@label}}</label>
      <div class="cds--select-input__wrapper">
        <select
          id={{@id}}
          class="cds--select-input"
          {{on "change" (inputValue @onChange)}}
        >
          {{#each @items as |i|}}
            <option
              value={{i.id}}
              selected={{eq i.id @value}}
            >{{i.label}}</option>
          {{/each}}
        </select>
        <Icon @name="chevron-down" class="cds--select__arrow" />
      </div>
    </div>
  </div>
</template>;

// Carbon DatePicker look on a native date input (no flatpickr).
export const DateField: TOC<{
  Args: {
    id: string;
    label: string;
    value: string;
    onChange: (v: string) => void;
  };
}> = <template>
  <div class="cds--form-item">
    <div class="cds--date-picker cds--date-picker--single">
      <div class="cds--date-picker-container">
        <label for={{@id}} class="cds--label">{{@label}}</label>
        <div class="cds--date-picker-input__wrapper">
          <input
            id={{@id}}
            type="date"
            class="cds--date-picker__input dis-date"
            value={{@value}}
            {{on "change" (inputValue @onChange)}}
          />
        </div>
      </div>
    </div>
  </div>
</template>;
