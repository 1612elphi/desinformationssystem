import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import './tag.css';

const markStyle = (mark?: string) =>
  mark ? htmlSafe(`--dis-tag-mark: ${mark}`) : undefined;

// A hairline label; @mark (a CSS colour, the party colour) draws a square before it.
const Tag: TOC<{
  Element: HTMLElement;
  Args: { mark?: string; onClick?: () => void };
  Blocks: { default: [] };
}> = <template>
  {{#if @onClick}}
    <button
      type="button"
      class="dis-tag dis-tag--button {{if @mark 'dis-tag--marked'}}"
      style={{markStyle @mark}}
      {{on "click" @onClick}}
      ...attributes
    >{{yield}}</button>
  {{else}}
    <span
      class="dis-tag {{if @mark 'dis-tag--marked'}}"
      style={{markStyle @mark}}
      ...attributes
    >{{yield}}</span>
  {{/if}}
</template>;

export default Tag;
