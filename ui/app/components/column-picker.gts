import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import CanonButton from 'delphicanon/components/canon-button';
import CanonPopover from 'delphicanon/components/canon-popover';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import CanonSwitch from 'delphicanon/components/canon-switch';
import Icon from 'ui/components/icon';
import './column-picker.css';

interface Signature {
  Args: {
    columns: { key: string; label: string }[];
    selected: string[];
    onChange: (keys: string[]) => void;
  };
}

const isFirst = (i: number) => i === 0;

export default class ColumnPicker extends Component<Signature> {
  isOn = (key: string) => this.args.selected.includes(key);

  isLast = (key: string) => this.args.selected.length === 1 && this.isOn(key);

  get allOn() {
    return this.args.columns.every((c) => this.isOn(c.key));
  }

  get triggerText() {
    return `Spalten (${this.args.selected.length})`;
  }

  toggle = (key: string, on: boolean) =>
    this.args.onChange(
      this.args.columns
        .map((c) => c.key)
        .filter((k) => (k === key ? on : this.isOn(k))),
    );

  showAll = () => this.args.onChange(this.args.columns.map((c) => c.key));

  <template>
    <CanonPopover
      class="dis-colpick"
      role="dialog"
      aria-label="Spalten"
      @placement="bottom-end"
    >
      <:trigger as |popover|>
        <CanonButton
          {{popover.trigger}}
          @kind="cell"
          @text={{this.triggerText}}
          @icon={{component Icon name="docs-columns"}}
        />
      </:trigger>
      <:content>
        <CanonRows>
          {{#each @columns as |c i|}}
            <CanonRow @title={{c.label}} as |titleId|>
              {{! popover focus goes to its autofocus element on open }}
              {{! template-lint-disable no-autofocus-attribute }}
              <CanonSwitch
                aria-labelledby={{titleId}}
                autofocus={{isFirst i}}
                @on={{this.isOn c.key}}
                @disabled={{this.isLast c.key}}
                @onChange={{fn this.toggle c.key}}
              />
              {{! template-lint-enable no-autofocus-attribute }}
            </CanonRow>
          {{/each}}
          <CanonButton
            @kind="secondary"
            @text="Alle Spalten"
            @icon={{component Icon name="docs-show"}}
            @disabled={{this.allOn}}
            @onClick={{this.showAll}}
          />
        </CanonRows>
      </:content>
    </CanonPopover>
  </template>
}
