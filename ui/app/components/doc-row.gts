import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import type { Doc } from 'ui/api';
import { SubmitterChips } from 'ui/components/party-chip';
import Tag from 'ui/components/tag';

// One document line in the meeting agenda and the Vorlage timeline.
const DocRow: TOC<{
  Args: { doc: Doc; onOpen: () => void; submitters?: boolean };
}> = <template>
  <button type="button" class="dis-meeting-docrow" {{on "click" @onOpen}}>
    <span class="dis-meeting-docrow__type"><Tag
        @type="gray"
        @size="sm"
      >{{@doc.doc_type}}</Tag></span>
    <span class="dis-meeting-docrow__label">{{@doc.label}}</span>
    {{#if @submitters}}
      {{#if @doc.submitters.length}}
        <span class="dis-meeting-docrow__sub"><SubmitterChips
            @codes={{@doc.submitters}}
          /></span>
      {{/if}}
    {{/if}}
  </button>
</template>;

export default DocRow;
