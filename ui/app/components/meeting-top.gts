import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import type { Doc, Vote } from 'ui/api';
import DocRow from 'ui/components/doc-row';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import SpeechList from 'ui/components/speech-list';
import Tag from 'ui/components/tag';
import VoteBar from 'ui/components/vote-bar';
import VoteBlock, { type Outcome } from 'ui/components/vote-block';
import type { Speech } from 'ui/utils/api-reden';
import type Load from 'ui/utils/load';
import { num } from 'ui/utils/meta';
import './meeting-top.css';

export interface TopItem {
  key: string;
  dom: string;
  number?: string;
  title?: string;
  sub: boolean;
  docs: Doc[];
  vote?: Vote;
  outcome?: Outcome;
  speeches: Speech[];
  speechCount: number;
  nonPublic: string;
  expandable: boolean;
  kind: 'contested' | 'vote' | 'docs' | 'none';
}

const headId = (item: TopItem) => `${item.dom}-head`;
const bodyId = (item: TopItem) => `${item.dom}-body`;

// One agenda item on the meeting timeline; the head toggles the body.
const MeetingTop: TOC<{
  Args: {
    item: TopItem;
    open: boolean;
    current: boolean;
    onToggle: () => void;
    onFocus: () => void;
    onOpenDoc: (doc: Doc) => void;
    speechLoad?: Load<Speech[]>;
    personIds?: Map<string, string>;
    onOpenPerson: (id: string) => void;
  };
}> = <template>
  <li
    class="dis-mtop dis-mtop--{{@item.kind}}
      {{if @item.sub 'dis-mtop--sub'}}
      {{if @open 'dis-mtop--open'}}
      {{if @current 'dis-mtop--current'}}"
    id={{@item.dom}}
  >
    <span class="dis-mtop-node" aria-hidden="true"></span>
    {{#if @item.expandable}}
      <button
        type="button"
        class="dis-mtop-head"
        id={{headId @item}}
        aria-expanded={{if @open "true" "false"}}
        aria-controls={{bodyId @item}}
        {{on "click" @onToggle}}
        {{on "focus" @onFocus}}
      >
        <span class="dis-mtop-num">{{if @item.number @item.number "—"}}</span>
        <span class="dis-mtop-title">{{if @item.title @item.title "—"}}
          {{#if @item.nonPublic}}<Tag>{{@item.nonPublic}}</Tag>{{/if}}</span>
        <span class="dis-mtop-meta">
          {{#if @item.outcome}}
            <VoteBar
              class="dis-mtop-bar"
              @compact={{true}}
              @ja={{@item.vote.ja}}
              @nein={{@item.vote.nein}}
              @enthaltung={{@item.vote.enthaltung}}
            />
            <span
              class="dis-mtop-outcome
                {{if @item.outcome.contested 'dis-mtop-outcome--contested'}}"
            >{{@item.outcome.label}}</span>
          {{/if}}
          {{#if @item.speechCount}}
            <span class="dis-mtop-docs" title="Wortbeiträge"><Icon
                @name="mic"
                @size={{12}}
              />{{num @item.speechCount}}</span>
          {{/if}}
          {{#if @item.docs.length}}
            <span class="dis-mtop-docs"><Icon
                @name="document"
                @size={{12}}
              />{{num @item.docs.length}}</span>
          {{/if}}
          <Icon @name="chevron-down" class="dis-mtop-arrow" />
        </span>
      </button>
    {{else}}
      <div
        class="dis-mtop-head dis-mtop-head--static"
        id={{headId @item}}
        tabindex="-1"
        {{on "focus" @onFocus}}
      >
        <span class="dis-mtop-num">{{if @item.number @item.number "—"}}</span>
        <span class="dis-mtop-title">{{if @item.title @item.title "—"}}
          {{#if @item.nonPublic}}<Tag>{{@item.nonPublic}}</Tag>{{/if}}</span>
      </div>
    {{/if}}
    {{#if @open}}
      <div class="dis-mtop-body" id={{bodyId @item}}>
        {{#if @item.docs.length}}
          <div class="dis-mtop-docs-list">
            {{#each @item.docs key="id" as |f|}}
              <DocRow
                @doc={{f}}
                @submitters={{true}}
                @onOpen={{fn @onOpenDoc f}}
              />
            {{/each}}
          </div>
        {{/if}}
        {{#if @item.vote}}<VoteBlock @vote={{@item.vote}} />{{/if}}
        {{#if @item.speechCount}}
          <LoadState @load={{@speechLoad}}>
            <SpeechList
              @speeches={{@item.speeches}}
              @heading={{true}}
              @personIds={{@personIds}}
              @onOpenPerson={{@onOpenPerson}}
            />
          </LoadState>
        {{/if}}
      </div>
    {{/if}}
  </li>
</template>;

export default MeetingTop;
