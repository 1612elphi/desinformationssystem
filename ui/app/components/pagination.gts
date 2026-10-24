import Component from '@glimmer/component';
import CanonButton from 'delphicanon/components/canon-button';
import Icon from 'ui/components/icon';
import { num } from 'ui/utils/meta';
import './pagination.css';

interface Signature {
  Args: {
    page: number;
    pageSize: number;
    total: number;
    onChange: (page: number) => void;
  };
}

export default class Pagination extends Component<Signature> {
  get pages() {
    return Math.max(1, Math.ceil(this.args.total / this.args.pageSize));
  }

  get position() {
    return `${num(this.args.page)} / ${num(this.pages)}`;
  }

  get multiple() {
    return this.pages > 1;
  }

  get atStart() {
    return this.args.page <= 1;
  }

  get atEnd() {
    return this.args.page >= this.pages;
  }

  prev = () => this.args.onChange(this.args.page - 1);
  next = () => this.args.onChange(this.args.page + 1);

  <template>
    {{#if this.multiple}}
      <nav class="dis-pagination" aria-label="Seiten">
        <CanonButton
          @kind="cell"
          @icon={{component Icon name="chevron-left"}}
          @label="Vorherige Seite"
          @disabled={{this.atStart}}
          @onClick={{this.prev}}
        />
        <span class="dis-pagination-pos" aria-live="polite">Seite
          {{this.position}}</span>
        <CanonButton
          @kind="cell"
          @icon={{component Icon name="chevron-right"}}
          @label="Nächste Seite"
          @disabled={{this.atEnd}}
          @onClick={{this.next}}
        />
      </nav>
    {{/if}}
  </template>
}
