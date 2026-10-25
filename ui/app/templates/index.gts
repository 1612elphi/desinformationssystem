import type { TOC } from '@ember/component/template-only';
import { pageTitle } from 'ember-page-title';
import DashLast from 'ui/components/dash-last';
import DashNext from 'ui/components/dash-next';
import DashQuick, { DashSearch } from 'ui/components/dash-quick';
import DashStats from 'ui/components/dash-stats';
import DocsWeek from 'ui/components/docs-week';
import type { IndexModel } from 'ui/routes/index';
import './index.css';

const Uebersicht: TOC<{ Args: { model: IndexModel } }> = <template>
  {{pageTitle "Übersicht"}}
  <div class="dis-dash">
    <h1 class="dis-visually-hidden">Übersicht</h1>
    <DashSearch />
    <div class="dis-dash-hero">
      <DashLast @load={{@model.last}} />
      <DashNext @load={{@model.next}} />
    </div>
    <DocsWeek />
    <div class="dis-dash-pair">
      <DashStats
        @stats={{@model.app.stats.value}}
        @error={{@model.app.stats.error}}
      />
      <DashQuick @lastId={{@model.last.value.last.meeting.id}} />
    </div>
  </div>
</template>;

export default Uebersicht;
