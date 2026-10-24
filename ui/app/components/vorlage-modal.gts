import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { getVorlage, type Vote } from 'ui/api';
import DocRow from 'ui/components/doc-row';
import { InlineLoading } from 'ui/components/loading';
import Modal from 'ui/components/modal';
import Tag from 'ui/components/tag';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { hasTally, tally } from 'ui/utils/meta';

const tallied = (votes: Vote[]) => votes.filter(hasTally);

interface Signature {
  Args: { nr: string; onClose: () => void };
}

export default class VorlageModal extends Component<Signature> {
  @service declare nav: NavService;

  @cached
  get load() {
    return new Load(getVorlage(this.args.nr));
  }

  get chain() {
    return this.load.value;
  }

  get heading() {
    return `Vorlage ${this.args.nr}`;
  }

  get stationsLabel() {
    const n = this.chain?.stations.length ?? 0;
    return `Werdegang (${n} Station${n === 1 ? '' : 'en'})`;
  }

  openDoc = (id: string) => this.nav.open({ vorlage: '', doc: id });

  <template>
    <Modal @heading={{this.heading}} @onClose={{@onClose}}>
      <div class="dis-detail">
        {{#if this.load.error}}
          <p class="dis-meta-line">Zu dieser Vorlagennummer ist (noch) nichts
            archiviert.</p>
        {{else if this.chain}}
          {{#if this.chain.title}}
            <p class="dis-vorlage-title">{{this.chain.title}}</p>
          {{/if}}
          <h4>{{this.stationsLabel}}</h4>
          <ol class="dis-timeline">
            {{#each this.chain.stations as |st|}}
              <li>
                <div class="dis-timeline__head">
                  <span class="dis-timeline__date">{{if
                      st.date
                      st.date
                      "—"
                    }}</span>
                  <span class="dis-timeline__body">{{if
                      st.body_name
                      st.body_name
                      "Unbekanntes Gremium"
                    }}</span>
                  {{#each (tallied st.votes) as |v|}}
                    <Tag
                      @type="green"
                      @size="sm"
                      title={{if
                        v.result_text
                        v.result_text
                        "Abstimmungsergebnis"
                      }}
                    >{{tally v}}</Tag>
                  {{/each}}
                </div>
                <div class="dis-timeline__docs">
                  {{#each st.documents key="id" as |d|}}
                    <DocRow @doc={{d}} @onOpen={{fn this.openDoc d.id}} />
                  {{/each}}
                </div>
              </li>
            {{/each}}
          </ol>
          <p class="dis-meta-line">
            Stationen = Sitzungen, in deren Dokumenten diese Vorlagennummer
            vorkommt (Vorberatung im Ausschuss bis Entscheidung im Gemeinderat).
          </p>
        {{else}}
          <InlineLoading @text="Lädt…" />
        {{/if}}
      </div>
    </Modal>
  </template>
}
