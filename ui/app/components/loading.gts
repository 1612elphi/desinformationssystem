import type { TOC } from '@ember/component/template-only';

const range = (n: number) => Array.from({ length: n }, (_, i) => i);

export const InlineLoading: TOC<{ Args: { text: string } }> = <template>
  <div class="cds--inline-loading" aria-live="assertive">
    <div class="cds--inline-loading__animation">
      <div class="cds--loading cds--loading--small">
        <svg class="cds--loading__svg" viewBox="0 0 100 100">
          <circle class="cds--loading__background" cx="50%" cy="50%" r="42" />
          <circle class="cds--loading__stroke" cx="50%" cy="50%" r="42" />
        </svg>
      </div>
    </div>
    <div class="cds--inline-loading__text">{{@text}}</div>
  </div>
</template>;

export const TableSkeleton: TOC<{ Args: { columns: number; rows: number } }> =
  <template>
    <div class="cds--skeleton cds--data-table-container">
      <table class="cds--skeleton cds--data-table cds--data-table--zebra">
        <thead>
          <tr>
            {{#each (range @columns)}}
              <th><span></span></th>
            {{/each}}
          </tr>
        </thead>
        <tbody>
          {{#each (range @rows)}}
            <tr>
              {{#each (range @columns)}}
                <td><span></span></td>
              {{/each}}
            </tr>
          {{/each}}
        </tbody>
      </table>
    </div>
  </template>;
