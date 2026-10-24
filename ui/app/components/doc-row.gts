import type { TOC } from '@ember/component/template-only';
import { on } from '@ember/modifier';
import type { Doc } from 'ui/api';
import { SubmitterChips } from 'ui/components/party-chip';
import Tag from 'ui/components/tag';
import './doc-row.css';

// One document line in the meeting agenda and the Vorlage timeline.
const DocRow: TOC<{
  Args: { doc: Doc; onOpen: () => void; submitters?: boolean };
}> = <template>
  <button type="button" class="dis-docrow" {{on "click" @onOpen}}>
    {{#if @doc.doc_type}}<Tag>{{@doc.doc_type}}</Tag>{{/if}}
    <span class="dis-docrow-label">{{@doc.label}}</span>
    {{#if @submitters}}
      <SubmitterChips @codes={{@doc.submitters}} />
    {{/if}}
  </button>
</template>;

export default DocRow;
