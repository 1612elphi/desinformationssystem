import type { TOC } from '@ember/component/template-only';
import Tag from 'ui/components/tag';
import { SUBMITTERS } from 'ui/utils/meta';

const meta = (code?: string) => (code ? SUBMITTERS[code] : undefined);

export const SubmitterChips: TOC<{ Args: { codes?: string[] } }> = <template>
  {{#if @codes.length}}
    <span class="dis-tags dis-tags--inline">
      {{#each @codes as |c|}}
        {{#let (meta c) as |m|}}
          <span title={{if m m.name c}}>
            <Tag @type={{if m m.colour "cool-gray"}} @size="sm">{{c}}</Tag>
          </span>
        {{/let}}
      {{/each}}
    </span>
  {{/if}}
</template>;

const PartyChip: TOC<{ Args: { code?: string; party?: string } }> = <template>
  {{#let (meta @code) as |m|}}
    {{#if m}}
      <span title={{m.name}}><Tag
          @type={{m.colour}}
          @size="sm"
        >{{@code}}</Tag></span>
    {{else if @party}}
      <Tag @type="cool-gray" @size="sm">{{@party}}</Tag>
    {{/if}}
  {{/let}}
</template>;

export default PartyChip;
