import Component from '@glimmer/component';
import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import CanonMenuCell from 'delphicanon/components/canon-menu-cell';
import CanonRow from 'delphicanon/components/canon-row';
import { inputValue } from 'ui/utils/meta';
import './fields.css';

export interface Item {
  id: string;
  label: string;
}

interface SelectSignature {
  Args: {
    id?: string;
    label: string;
    items: Item[];
    value: string;
    onChange: (v: string) => void;
  };
}

// A labelled CanonMenuCell; a value not among the items shows as the placeholder.
export class SelectField extends Component<SelectSignature> {
  get labels() {
    return this.args.items.map((i) => i.label);
  }

  get selected() {
    const i = this.args.items.findIndex((x) => x.id === this.args.value);
    return i < 0 ? undefined : i;
  }

  get placeholder() {
    return this.args.value || '—';
  }

  pick = (index: number) =>
    this.args.onChange(this.args.items[index]?.id ?? '');

  <template>
    <CanonRow class="dis-field" @title={{@label}} @form="stacked" as |titleId|>
      <CanonMenuCell
        @items={{this.labels}}
        @selected={{this.selected}}
        @placeholder={{this.placeholder}}
        @labelledBy={{titleId}}
        @onChange={{this.pick}}
      />
    </CanonRow>
  </template>
}

export const DateField: TOC<{
  Args: {
    id: string;
    label: string;
    value: string;
    onChange: (v: string) => void;
  };
}> = <template>
  <CanonRow class="dis-field" @title={{@label}} @form="stacked" as |titleId|>
    <input
      id={{@id}}
      type="date"
      class="dis-date"
      aria-labelledby={{titleId}}
      value={{@value}}
      {{on "change" (inputValue @onChange)}}
    />
  </CanonRow>
</template>;
