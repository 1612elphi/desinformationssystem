import type { TOC } from '@ember/component/template-only';
import { htmlSafe } from '@ember/template';
import { num } from 'ui/utils/meta';
import './vote-bar.css';

const share = (n: number | null | undefined, total: number) =>
  htmlSafe(`flex-grow: ${total ? (n ?? 0) / total : 0}`);
const sum = (...xs: (number | null | undefined)[]) =>
  xs.reduce<number>((a, x) => a + (x ?? 0), 0);
const label = (
  ja?: number | null,
  nein?: number | null,
  enth?: number | null,
) => `Ja ${num(ja)}, Nein ${num(nein)}, Enthaltung ${num(enth)}`;

// Ja, Nein and Enthaltung as one stacked bar; @compact drops the numbers.
const VoteBar: TOC<{
  Element: HTMLDivElement;
  Args: {
    ja?: number | null;
    nein?: number | null;
    enthaltung?: number | null;
    compact?: boolean;
  };
}> = <template>
  {{#let (sum @ja @nein @enthaltung) as |total|}}
    <div
      class="dis-votebar {{if @compact 'dis-votebar--compact'}}"
      role="img"
      aria-label={{label @ja @nein @enthaltung}}
      ...attributes
    >
      <span class="dis-votebar-track" aria-hidden="true">
        <span
          class="dis-votebar-seg dis-votebar-ja"
          style={{share @ja total}}
        ></span>
        <span
          class="dis-votebar-seg dis-votebar-nein"
          style={{share @nein total}}
        ></span>
        <span
          class="dis-votebar-seg dis-votebar-enth"
          style={{share @enthaltung total}}
        ></span>
      </span>
      {{#unless @compact}}
        <span class="dis-votebar-nums" aria-hidden="true">
          <span class="dis-votebar-n dis-votebar-ja-n">{{num @ja}}</span>
          <span class="dis-votebar-n dis-votebar-nein-n">{{num @nein}}</span>
          <span class="dis-votebar-n dis-votebar-enth-n">{{num
              @enthaltung
            }}</span>
        </span>
      {{/unless}}
    </div>
  {{/let}}
</template>;

export default VoteBar;
