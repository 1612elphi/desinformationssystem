import Component from '@glimmer/component';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import './data-table.css';

export interface Column {
  key: string;
  label: string;
  width?: string;
  cls?: string;
}

const widthStyle = (c: Column) =>
  c.width ? htmlSafe(`inline-size: ${c.width}`) : undefined;
const isFirst = (i: number) => i === 0;

interface Signature<T> {
  Element: HTMLDivElement;
  Args: {
    columns: Column[];
    rows: T[];
    caption?: string;
    onOpen?: (row: T) => void;
    rowLabel?: (row: T) => string;
  };
  Blocks: { default: [row: T, column: Column] };
}

export default class DataTable<T> extends Component<Signature<T>> {
  label = (row: T) => this.args.rowLabel?.(row) ?? '';
  open = (row: T) => this.args.onOpen?.(row);

  <template>
    <div class="dis-table-wrap" ...attributes>
      <table class="dis-table {{if @onOpen 'dis-table--open'}}">
        {{#if @caption}}<caption>{{@caption}}</caption>{{/if}}
        <thead>
          <tr>
            {{#each @columns as |c|}}
              <th
                scope="col"
                class={{c.cls}}
                style={{widthStyle c}}
              >{{c.label}}</th>
            {{/each}}
          </tr>
        </thead>
        <tbody>
          {{#each @rows as |row|}}
            <tr>
              {{#each @columns as |c i|}}
                <td class={{c.cls}} data-label={{c.label}}>
                  {{#if (isFirst i)}}
                    {{#if @onOpen}}
                      <button
                        type="button"
                        class="dis-table-open"
                        aria-label={{this.label row}}
                        {{on "click" (fn this.open row)}}
                      ></button>
                    {{/if}}
                  {{/if}}
                  {{yield row c}}
                </td>
              {{/each}}
            </tr>
          {{/each}}
        </tbody>
      </table>
    </div>
  </template>
}
