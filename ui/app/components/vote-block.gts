import type { TOC } from '@ember/component/template-only';
import type { Vote } from 'ui/api';
import Icon from 'ui/components/icon';
import { hasTally } from 'ui/utils/meta';
import './vote-block.css';

const dash = (n?: number | null) => n ?? '—';
const voteClass = (v?: string) => (v || '').replace(/[^a-z]/g, '');
const isHttp = (u?: string) => !!u?.startsWith('http');

const VoteBlock: TOC<{ Args: { vote: Vote } }> = <template>
  <div class="dis-vote">
    {{#if @vote.result_text}}
      <div class="dis-vote__result">Ergebnis: {{@vote.result_text}}</div>
    {{/if}}
    {{#if (hasTally @vote)}}
      <div class="dis-vote__tally">
        <span class="dis-vote__n dis-vote__ja">Ja {{dash @vote.ja}}</span>
        <span class="dis-vote__n dis-vote__nein">Nein {{dash @vote.nein}}</span>
        <span class="dis-vote__n dis-vote__enth">Enthaltung
          {{dash @vote.enthaltung}}</span>
      </div>
    {{/if}}
    {{#if @vote.members.length}}
      <details class="dis-vote__roll">
        <summary>Namentliche Abstimmung ({{@vote.members.length}})</summary>
        <div class="dis-vote__members">
          {{#each @vote.members as |m|}}
            <span
              class="dis-vote__m dis-vote__m--{{voteClass m.vote}}"
              title={{m.vote}}
            >{{m.name}}</span>
          {{/each}}
        </div>
      </details>
    {{/if}}
    {{#if (isHttp @vote.image_url)}}
      <a
        class="dis-vote__image"
        href={{@vote.image_url}}
        target="_blank"
        rel="noopener noreferrer"
      >
        Abstimmungsbild
        <Icon @name="launch" @size={{12}} />
      </a>
    {{/if}}
  </div>
</template>;

export default VoteBlock;
