import type { TOC } from '@ember/component/template-only';

// FTS snippet highlighting: the backend delimits matches with control chars
// (\x01…\x02), never markup, so document text cannot inject HTML.
function parts(text: string) {
  return text.split('\u0001').map((seg, i) => {
    if (i === 0) return { hl: '', rest: seg };
    const cut = seg.indexOf('\u0002');
    return cut >= 0
      ? { hl: seg.slice(0, cut), rest: seg.slice(cut + 1) }
      : { hl: seg, rest: '' };
  });
}

const Snippet: TOC<{ Args: { text: string } }> = <template>
  <span class="dis-snippet">
    {{~#each (parts @text) as |p|~}}
      {{~#if p.hl}}<mark>{{p.hl}}</mark>{{/if}}{{p.rest~}}
    {{~/each~}}
  </span>
</template>;

export default Snippet;
