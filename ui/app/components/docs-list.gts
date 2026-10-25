import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import type { Doc } from 'ui/api';
import DocsGlyph from 'ui/components/docs-glyph';
import Icon from 'ui/components/icon';
import { SubmitterChips } from 'ui/components/party-chip';
import Snippet from 'ui/components/snippet';
import Tag from 'ui/components/tag';
import { relDate } from 'ui/utils/api-dokumente';
import { eq } from 'ui/utils/meta';
import './docs-list.css';

const title = (d: Doc) => d.agenda_title || d.label;
const sub = (d: Doc) =>
  [
    d.doc_type,
    d.agenda_title && d.label !== d.doc_type ? d.label : '',
    d.agenda_number ? `TOP ${d.agenda_number}` : '',
  ]
    .filter(Boolean)
    .join(' · ');

const DocsList: TOC<{
  Args: {
    docs: Doc[];
    cursor: number;
    onOpen: (d: Doc) => void;
    onFocusRow: (i: number) => void;
  };
}> = <template>
  <ol class="dis-dlist">
    {{#each @docs key="id" as |d i|}}
      <li class="dis-dlist-row {{if (eq i @cursor) 'is-cursor'}}">
        <DocsGlyph @type={{d.doc_type}} />
        <div class="dis-dlist-main">
          <button
            type="button"
            class="dis-dlist-open"
            {{on "click" (fn @onOpen d)}}
            {{on "focus" (fn @onFocusRow i)}}
          >{{title d}}</button>
          <span class="dis-dlist-sub">{{sub d}}</span>
          {{#if d.snippet}}<Snippet @text={{d.snippet}} />{{/if}}
        </div>
        <div class="dis-dlist-meta">
          <span class="dis-dlist-when">
            {{#if d.meeting_date}}
              <time
                class="dis-dlist-date"
                datetime={{d.meeting_date}}
                title={{d.meeting_date}}
              >{{relDate d.meeting_date}}</time>
            {{/if}}
            {{#if d.body_name}}
              <span class="dis-dlist-body"><Icon
                  @name="docs-body"
                  @size={{12}}
                />{{d.body_name}}</span>
            {{/if}}
          </span>
          <span class="dis-tags">
            {{#if d.category}}<Tag>{{d.category}}</Tag>{{/if}}
            <SubmitterChips @codes={{d.submitters}} />
          </span>
        </div>
      </li>
    {{/each}}
  </ol>
</template>;

export default DocsList;
