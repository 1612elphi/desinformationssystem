import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import DocsGlyph from 'ui/components/docs-glyph';
import Icon from 'ui/components/icon';
import type { FacetCount } from 'ui/utils/api-dokumente';
import { eq, num } from 'ui/utils/meta';
import './docs-facets.css';

const TOP = 6;

// The top values, plus the selected one when it ranks lower.
const top = (items: FacetCount[] | undefined, value: string) => {
  const all = items ?? [];
  const head = all.slice(0, TOP);
  const picked = all.find((f) => f.value === value);
  return picked && !head.includes(picked) ? [...head, picked] : head;
};

const toggled = (value: string, current: string) =>
  value === current ? '' : value;

const FacetGroup: TOC<{
  Args: {
    label: string;
    icon: string;
    items?: FacetCount[];
    value: string;
    glyph?: boolean;
    onPick: (value: string) => void;
  };
}> = <template>
  {{#if @items.length}}
    <div class="dis-facets-group" role="group" aria-label={{@label}}>
      <span class="dis-facets-label"><Icon
          @name={{@icon}}
          @size={{14}}
        />{{@label}}</span>
      {{#each (top @items @value) as |f|}}
        <button
          type="button"
          class="dis-facet"
          aria-pressed={{if (eq f.value @value) "true" "false"}}
          {{on "click" (fn @onPick (toggled f.value @value))}}
        >
          {{#if @glyph}}<DocsGlyph @type={{f.value}} />{{/if}}
          <span class="dis-facet-name">{{f.value}}</span>
          <span class="dis-facet-n">{{num f.n}}</span>
        </button>
      {{/each}}
    </div>
  {{/if}}
</template>;

// Quick filters with match counts for the current search.
const DocsFacets: TOC<{
  Args: {
    categories?: FacetCount[];
    types?: FacetCount[];
    category: string;
    type: string;
    onCategory: (value: string) => void;
    onType: (value: string) => void;
  };
}> = <template>
  <div class="dis-facets">
    <FacetGroup
      @label="Kategorie"
      @icon="docs-category"
      @items={{@categories}}
      @value={{@category}}
      @onPick={{@onCategory}}
    />
    <FacetGroup
      @label="Typ"
      @icon="docs-type"
      @items={{@types}}
      @value={{@type}}
      @glyph={{true}}
      @onPick={{@onType}}
    />
  </div>
</template>;

export default DocsFacets;
