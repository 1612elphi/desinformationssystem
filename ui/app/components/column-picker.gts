import Component from '@glimmer/component';
import { tracked } from '@glimmer/tracking';
import { on } from '@ember/modifier';
import { fn } from '@ember/helper';
import Icon from 'ui/components/icon';
import { clickOutside } from 'ui/utils/modifiers';

export interface Column {
  key: string;
  label: string;
  cls: string;
}

interface Signature {
  Args: {
    columns: Column[];
    selected: string[];
    onChange: (keys: string[]) => void;
  };
}

// Carbon MultiSelect look; an empty selection falls back to all columns.
export default class ColumnPicker extends Component<Signature> {
  @tracked open = false;

  toggleOpen = () => (this.open = !this.open);
  close = () => (this.open = false);

  isOn = (key: string) => this.args.selected.includes(key);

  toggle = (key: string) => {
    const wasOn = this.isOn(key);
    this.change(
      this.args.columns
        .map((c) => c.key)
        .filter((k) => (k === key ? !wasOn : this.isOn(k))),
    );
  };

  clear = () => this.change([]);

  change(keys: string[]) {
    this.args.onChange(
      keys.length ? keys : this.args.columns.map((c) => c.key),
    );
  }

  <template>
    <div
      class="cds--multi-select__wrapper cds--list-box__wrapper"
      {{clickOutside this.close}}
    >
      <div
        class="cds--multi-select cds--list-box cds--list-box--sm cds--multi-select--selected
          {{if this.open 'cds--list-box--expanded'}}"
      >
        <div class="cds--list-box__field">
          <div class="cds--tag cds--tag--filter cds--tag--high-contrast">
            <span class="cds--tag__label">{{@selected.length}}</span>
            <button
              type="button"
              class="cds--tag__close-icon"
              aria-label="Clear all selected items"
              {{on "click" this.clear}}
            >
              <Icon @name="close" />
            </button>
          </div>
          <button
            type="button"
            class="dis-colpick-toggle"
            aria-expanded={{if this.open "true" "false"}}
            {{on "click" this.toggleOpen}}
          >
            <span class="cds--list-box__label">Spalten</span>
            <span
              class="cds--list-box__menu-icon
                {{if this.open 'cds--list-box__menu-icon--open'}}"
            >
              <Icon @name="chevron-down" />
            </span>
          </button>
        </div>
        {{#if this.open}}
          <ul class="cds--list-box__menu">
            {{#each @columns as |c|}}
              <li class="cds--list-box__menu-item">
                <div class="cds--list-box__menu-item__option">
                  <div class="cds--form-item cds--checkbox-wrapper">
                    <input
                      type="checkbox"
                      class="cds--checkbox"
                      id="dis-col-{{c.key}}"
                      checked={{this.isOn c.key}}
                      {{on "change" (fn this.toggle c.key)}}
                    />
                    <label for="dis-col-{{c.key}}" class="cds--checkbox-label">
                      <span class="cds--checkbox-label-text">{{c.label}}</span>
                    </label>
                  </div>
                </div>
              </li>
            {{/each}}
          </ul>
        {{/if}}
      </div>
    </div>
  </template>
}
