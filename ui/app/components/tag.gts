import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';

const Tag: TOC<{
  Element: HTMLElement;
  Args: { type?: string; size?: 'sm'; onClick?: () => void };
  Blocks: { default: [] };
}> = <template>
  {{#if @onClick}}
    <button
      type="button"
      class="cds--tag cds--tag--{{if @type @type 'gray'}}
        {{if @size 'cds--tag--sm'}}
        dis-tag-button"
      {{on "click" @onClick}}
      ...attributes
    ><span class="cds--tag__label">{{yield}}</span></button>
  {{else}}
    <span
      class="cds--tag cds--tag--{{if @type @type 'gray'}}
        {{if @size 'cds--tag--sm'}}"
      ...attributes
    ><span class="cds--tag__label">{{yield}}</span></span>
  {{/if}}
</template>;

export default Tag;
