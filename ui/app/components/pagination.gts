import Component from '@glimmer/component';
import { on } from '@ember/modifier';
import Icon from 'ui/components/icon';
import { eq } from 'ui/utils/meta';

interface Signature {
  Args: {
    page: number;
    pageSize: number;
    total: number;
    onChange: (page: number) => void;
  };
}

// Markup and default English texts of @carbon/react's Pagination.
export default class Pagination extends Component<Signature> {
  get pages() {
    return Math.max(1, Math.ceil(this.args.total / this.args.pageSize));
  }

  get pageList() {
    return Array.from({ length: this.pages }, (_, i) => i + 1);
  }

  get rangeText() {
    const { page, pageSize, total } = this.args;
    const min = total ? (page - 1) * pageSize + 1 : 0;
    const max = Math.min(page * pageSize, total);
    return `${min}–${max} of ${total} item${total === 1 ? '' : 's'}`;
  }

  get pagesText() {
    return `of ${this.pages} page${this.pages === 1 ? '' : 's'}`;
  }

  get atStart() {
    return this.args.page <= 1;
  }

  get atEnd() {
    return this.args.page >= this.pages;
  }

  prev = () => this.args.onChange(this.args.page - 1);
  next = () => this.args.onChange(this.args.page + 1);

  pick = (e: Event) =>
    this.args.onChange(Number((e.target as HTMLSelectElement).value));

  <template>
    <div class="cds--pagination">
      <div class="cds--pagination__left">
        <label class="cds--pagination__text" for="dis-page-size">Items per page:</label>
        <div class="cds--select cds--select--inline cds--select__item-count">
          <div class="cds--select-input--inline__wrapper">
            <div class="cds--select-input__wrapper">
              <select id="dis-page-size" class="cds--select-input">
                <option value={{@pageSize}}>{{@pageSize}}</option>
              </select>
              <Icon @name="chevron-down" class="cds--select__arrow" />
            </div>
          </div>
        </div>
        <span
          class="cds--pagination__text cds--pagination__items-count"
        >{{this.rangeText}}</span>
      </div>
      <div class="cds--pagination__right">
        <div class="cds--select cds--select--inline cds--select__page-number">
          <div class="cds--select-input--inline__wrapper">
            <div class="cds--select-input__wrapper">
              <select
                class="cds--select-input"
                aria-label="Page Number"
                {{on "change" this.pick}}
              >
                {{#each this.pageList as |p|}}
                  <option value={{p}} selected={{eq p @page}}>{{p}}</option>
                {{/each}}
              </select>
              <Icon @name="chevron-down" class="cds--select__arrow" />
            </div>
          </div>
        </div>
        <span class="cds--pagination__text">{{this.pagesText}}</span>
        <div class="cds--pagination__control-buttons">
          <button
            type="button"
            class="cds--btn cds--btn--ghost cds--btn--icon-only cds--pagination__button cds--pagination__button--backward
              {{if this.atStart 'cds--pagination__button--no-index'}}"
            aria-label="Previous page"
            disabled={{this.atStart}}
            {{on "click" this.prev}}
          >
            <Icon @name="caret-left" class="cds--btn__icon" />
          </button>
          <button
            type="button"
            class="cds--btn cds--btn--ghost cds--btn--icon-only cds--pagination__button cds--pagination__button--forward
              {{if this.atEnd 'cds--pagination__button--no-index'}}"
            aria-label="Next page"
            disabled={{this.atEnd}}
            {{on "click" this.next}}
          >
            <Icon @name="caret-right" class="cds--btn__icon" />
          </button>
        </div>
      </div>
    </div>
  </template>
}
