import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { htmlSafe } from '@ember/template';
import CanonButton from 'delphicanon/components/canon-button';
import Avatar from 'ui/components/avatar';
import Icon from 'ui/components/icon';
import PartyChip from 'ui/components/party-chip';
import { partyMark } from 'ui/utils/api-menschen';
import { clock, type Speech } from 'ui/utils/api-reden';
import { num } from 'ui/utils/meta';
import './speech-list.css';

const words = (s: Speech) => s.text.split(/\s+/).filter(Boolean).length;
const paragraphs = (s: Speech) => s.text.split(/\n\s*\n/);
const segment = (s: Speech) =>
  htmlSafe(
    `flex-grow: ${Math.max(1, s.end_s - s.start_s)}; background: ${partyMark(s.party_code)}`,
  );
const who = (s: Speech) => s.speaker || 'Unbekannt';
const lookup = (ids: Map<string, string> | undefined, s: Speech) =>
  s.speaker ? ids?.get(s.speaker) : undefined;

const SpeechList: TOC<{
  Args: {
    speeches: Speech[];
    heading?: boolean;
    personIds?: Map<string, string>;
    onOpenPerson: (id: string) => void;
  };
}> = <template>
  {{#if @speeches.length}}
    <section class="dis-speeches">
      {{#if @heading}}
        <h5 class="dis-speeches-head"><Icon
            @name="mic"
            @size={{14}}
          />Wortbeiträge ({{num @speeches.length}})</h5>
      {{/if}}
      <div class="dis-speeches-strip" aria-hidden="true">
        {{#each @speeches key="id" as |s|}}
          <span style={{segment s}} title={{who s}}></span>
        {{/each}}
      </div>
      {{#each @speeches key="id" as |s|}}
        {{#let (lookup @personIds s) as |id|}}
          <details class="dis-speech">
            <summary>
              <Avatar
                class="dis-speech-avatar"
                @personId={{id}}
                @name={{who s}}
              />
              <span class="dis-speech-who">{{who s}}</span>
              {{#if s.party_code}}
                <PartyChip @code={{s.party_code}} @party={{s.party_code}} />
              {{/if}}
              <span class="dis-speech-meta"><Icon
                  @name="time"
                  @size={{12}}
                />{{clock s.start_s}}
                ·
                {{num (words s)}}
                Wörter</span>
            </summary>
            <div class="dis-speech-body">
              {{#each (paragraphs s) as |para|}}
                <p>{{para}}</p>
              {{/each}}
              {{#if id}}
                <CanonButton
                  class="dis-speech-person"
                  @kind="cell"
                  @text="Profil"
                  @icon={{component Icon name="users"}}
                  @onClick={{fn @onOpenPerson id}}
                />
              {{/if}}
            </div>
          </details>
        {{/let}}
      {{/each}}
    </section>
  {{/if}}
</template>;

export default SpeechList;
