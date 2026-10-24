import type { TOC } from '@ember/component/template-only';
import { concat } from '@ember/helper';
import CanonLoader from 'delphicanon/components/canon-loader';
import CanonStatus from 'delphicanon/components/canon-status';
import type Load from 'ui/utils/load';
import './loading.css';

export const InlineLoading: TOC<{ Args: { text: string } }> = <template>
  <div class="dis-loading" role="status">
    <CanonLoader />
    <span>{{@text}}</span>
  </div>
</template>;

export const LoadError: TOC<{ Args: { error: string } }> = <template>
  <div role="alert">
    <CanonStatus
      @tone="warn"
      @row={{true}}
      @text={{concat "Fehler: " @error}}
    />
  </div>
</template>;

// Loader until the first value, the error if the request failed, else the value.
export const LoadState: TOC<{
  Args: { load: Load<unknown> | undefined; text?: string };
  Blocks: { default: [] };
}> = <template>
  {{#if @load.error}}
    <LoadError @error={{@load.error}} />
  {{else if @load.value}}
    {{yield}}
  {{else}}
    <InlineLoading @text={{if @text @text "Lädt…"}} />
  {{/if}}
</template>;
