import type { TOC } from '@ember/component/template-only';
import { htmlSafe } from '@ember/template';
import rough from 'ui/modifiers/rough';
import { partyMark, partyName } from 'ui/utils/api-menschen';
import { SEAT_ORDER, num } from 'ui/utils/meta';
import './gremium-share.css';

type Part = { code: string; n: number };

const rank = (code: string) => {
  const i = SEAT_ORDER.indexOf(code);
  return i < 0 ? SEAT_ORDER.length : i;
};
const ordered = (parts: Part[] = []) =>
  [...parts].sort((a, b) => rank(a.code) - rank(b.code));
const total = (parts: Part[] = []) => parts.reduce((a, p) => a + p.n, 0);
const isLargest = (p: Part, parts: Part[] = []) =>
  parts.length > 1 && p.n === Math.max(...parts.map((x) => x.n));
const segStyle = (p: Part) =>
  htmlSafe(`flex-grow: ${p.n}; background: ${partyMark(p.code)}`);
const markStyle = (p: Part) =>
  htmlSafe(`--dis-share-mark: ${partyMark(p.code)}`);
const pctOf = (n: number, all: number) =>
  all ? `${Math.round((n / all) * 100)} %` : '—';
const label = (parts?: Part[]) =>
  ordered(parts)
    .map((p) => `${p.code || '–'} ${num(p.n)}`)
    .join(', ');

const GremiumShare: TOC<{
  Element: HTMLDivElement;
  Args: { parts?: Part[] };
}> = <template>
  {{#let (total @parts) as |all|}}
    <div class="dis-share" ...attributes>
      <div class="dis-share-bar" role="img" aria-label={{label @parts}}>
        {{#each (ordered @parts) as |p|}}
          <span class="dis-share-seg" style={{segStyle p}}></span>
        {{/each}}
      </div>
      <ul class="dis-share-legend">
        {{#each (ordered @parts) as |p|}}
          <li style={{markStyle p}} title={{partyName p.code}}>
            <span
              class="dis-share-code"
              {{rough
                "underline"
                show=(isLargest p @parts)
                color="--dis-highlight"
                strokeWidth=2
                padding=1
                delay=700
              }}
            >{{if p.code p.code "–"}}</span>
            <span class="dis-share-n">{{num p.n}}</span>
            <span class="dis-share-pct">{{pctOf p.n all}}</span>
          </li>
        {{/each}}
      </ul>
    </div>
  {{/let}}
</template>;

export default GremiumShare;
