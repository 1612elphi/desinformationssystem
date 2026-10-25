import Component from '@glimmer/component';
import { cached } from '@glimmer/tracking';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import CanonRow from 'delphicanon/components/canon-row';
import CanonRows from 'delphicanon/components/canon-rows';
import { getVorlage, type Vote } from 'ui/api';
import DocRow from 'ui/components/doc-row';
import { LoadState } from 'ui/components/loading';
import Modal, { type PaneContext } from 'ui/components/modal';
import Icon from 'ui/components/icon';
import Tag from 'ui/components/tag';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import Load from 'ui/utils/load';
import { hasTally, tally } from 'ui/utils/meta';
import './vorlage-modal.css';

const tallied = (votes: Vote[]) => votes.filter(hasTally);
const decided = (i: number, n: number, votes: Vote[]) =>
  i === n - 1 && tallied(votes).length > 0;

interface Signature {
  Args: { nr: string; onClose: () => void; pane?: PaneContext };
}

export default class VorlageModal extends Component<Signature> {
  @service declare nav: NavService;

  @cached
  get load() {
    return this.args.nr ? new Load(getVorlage(this.args.nr)) : undefined;
  }

  get chain() {
    return this.load?.value;
  }

  get notFound() {
    return this.load?.error.startsWith('404') ?? false;
  }

  get heading() {
    return `Vorlage ${this.args.nr}`;
  }

  get stationsLabel() {
    const n = this.chain?.stations.length ?? 0;
    return `Werdegang (${n} Station${n === 1 ? '' : 'en'})`;
  }

  openDoc = (id: string) => this.nav.open({ doc: id });

  <template>
    <Modal @heading={{this.heading}} @pane={{@pane}} @onClose={{@onClose}}>
      {{#if this.notFound}}
        <p class="dis-vorlage-note"><Icon @name="docs-none" />Zu dieser
          Vorlagennummer ist (noch) nichts archiviert.</p>
      {{else}}
        <LoadState @load={{this.load}}>
          {{#if this.chain}}
            {{#let this.chain as |chain|}}
              {{#if chain.title}}
                <p class="dis-vorlage-title">{{chain.title}}</p>
              {{/if}}
              <h4 class="dis-vorlage-heading"><Icon
                  @name="docs-route"
                />{{this.stationsLabel}}</h4>
              <ol class="dis-vorlage-stations">
                {{#each chain.stations as |st i|}}
                  <li>
                    <CanonRows>
                      <CanonRow
                        @title={{if
                          st.body_name
                          st.body_name
                          "Unbekanntes Gremium"
                        }}
                        @detail={{if st.date st.date "—"}}
                      >
                        <span
                          class="dis-vorlage-tallies"
                          {{rough
                            "box"
                            color="--dis-yes"
                            padding=4
                            show=(decided i chain.stations.length st.votes)
                            delay=400
                          }}
                        >
                          {{#each (tallied st.votes) as |v|}}
                            <Tag
                              title={{if
                                v.result_text
                                v.result_text
                                "Abstimmungsergebnis"
                              }}
                            ><Icon @name="docs-vote" @size={{12}} />{{tally
                                v
                              }}</Tag>
                          {{/each}}
                        </span>
                      </CanonRow>
                      {{#if st.documents.length}}
                        <div>
                          {{#each st.documents key="id" as |d|}}
                            <DocRow
                              @doc={{d}}
                              @onOpen={{fn this.openDoc d.id}}
                            />
                          {{/each}}
                        </div>
                      {{/if}}
                    </CanonRows>
                  </li>
                {{/each}}
              </ol>
              <p class="dis-vorlage-note">
                <Icon @name="docs-info" />
                Stationen = Sitzungen, in deren Dokumenten diese Vorlagennummer
                vorkommt (Vorberatung im Ausschuss bis Entscheidung im
                Gemeinderat).
              </p>
            {{/let}}
          {{/if}}
        </LoadState>
      {{/if}}
    </Modal>
  </template>
}
