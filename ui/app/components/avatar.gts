import type { TOC } from '@ember/component/template-only';
import { initials } from 'ui/utils/api-menschen';
import { portrait } from 'ui/utils/chamber';
import './avatar.css';

const Avatar: TOC<{
  Element: HTMLSpanElement;
  Args: { personId?: string | null; name?: string };
}> = <template>
  <span class="dis-avatar" aria-hidden="true" ...attributes>
    {{#let (portrait @personId) as |src|}}
      {{#if src}}
        <img class="dis-avatar-photo" src={{src}} alt="" loading="lazy" />
      {{else}}
        {{initials @name}}
      {{/if}}
    {{/let}}
  </span>
</template>;

export default Avatar;
