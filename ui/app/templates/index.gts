import type { TOC } from '@ember/component/template-only';
import { pageTitle } from 'ember-page-title';
import type { ApplicationModel } from 'ui/routes/application';

const Uebersicht: TOC<{ Args: { model: ApplicationModel } }> = <template>
  {{pageTitle "Übersicht"}}
  <h1 class="dis-visually-hidden">Übersicht</h1>
</template>;

export default Uebersicht;
