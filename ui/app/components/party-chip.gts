import type { TOC } from '@ember/component/template-only';
import Tag from 'ui/components/tag';
import afdNote from 'ui/modifiers/afd-note';
import { SUBMITTERS } from 'ui/utils/meta';

const meta = (code?: string) => (code ? SUBMITTERS[code] : undefined);

export const SubmitterChips: TOC<{ Args: { codes?: string[] } }> = <template>
  {{#if @codes.length}}
    <span class="dis-tags">
      {{#each @codes as |c|}}
        {{#let (meta c) as |m|}}
          <Tag
            @mark={{m.mark}}
            title={{if m m.name c}}
            {{afdNote c}}
          >{{c}}</Tag>
        {{/let}}
      {{/each}}
    </span>
  {{/if}}
</template>;

const PartyChip: TOC<{ Args: { code?: string; party?: string } }> = <template>
  {{#let (meta @code) as |m|}}
    {{#if m}}
      <Tag @mark={{m.mark}} title={{m.name}} {{afdNote @code}}>{{@code}}</Tag>
    {{else if @party}}
      <Tag>{{@party}}</Tag>
    {{/if}}
  {{/let}}
</template>;

export default PartyChip;
