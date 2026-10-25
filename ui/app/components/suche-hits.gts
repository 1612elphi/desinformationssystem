import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import type { CommitteeListItem, Meeting } from 'ui/api';
import Avatar from 'ui/components/avatar';
import Icon from 'ui/components/icon';
import PartyChip from 'ui/components/party-chip';
import Snippet from 'ui/components/snippet';
import Tag from 'ui/components/tag';
import VoteBar from 'ui/components/vote-bar';
import { outcome, outcomeIcon } from 'ui/components/vote-block';
import rough from 'ui/modifiers/rough';
import { relDate } from 'ui/utils/api-dokumente';
import {
  highlight,
  type PersonHit,
  type SpeechHit,
  type VoteHit,
} from 'ui/utils/api-suche';
import { dateDe } from 'ui/utils/calendar';
import { num } from 'ui/utils/meta';
import './suche-hits.css';

const MONTH = new Intl.DateTimeFormat('de-DE', { month: 'short' });
const day = (iso?: string) => (iso ? String(Number(iso.slice(8, 10))) : '');
const month = (iso?: string) =>
  iso ? MONTH.format(new Date(`${iso.slice(0, 10)}T12:00:00`)) : '';
const year = (iso?: string) => iso?.slice(0, 4) ?? '';
const join = (...xs: (string | null | undefined)[]) =>
  xs.filter(Boolean).join(' · ');
const top = (n?: string | null) => (n ? `TOP ${n}` : '');
const outcomeOf = (v: VoteHit) => outcome({ ...v, id: '' });
const iconOf = (key?: string) => outcomeIcon(key ?? '');
const stagger = (i: number) => 500 + i * 120;
const personOf = (ids: Map<string, string>, name: string) => ids.get(name);

const DateTile: TOC<{ Args: { date?: string } }> = <template>
  <time class="dis-hit-tile" datetime={{@date}}>
    <span class="dis-hit-tile-day">{{day @date}}</span>
    <span class="dis-hit-tile-month">{{month @date}}</span>
    <span class="dis-hit-tile-year">{{year @date}}</span>
  </time>
</template>;

export const MeetingHits: TOC<{
  Args: { meetings: Meeting[]; onOpen: (id: string) => void };
}> = <template>
  <ol class="dis-hits">
    {{#each @meetings key="id" as |m|}}
      <li class="dis-hit">
        <DateTile @date={{m.date}} />
        <div class="dis-hit-main">
          <button
            type="button"
            class="dis-hit-open"
            {{on "click" (fn @onOpen m.id)}}
          >{{if m.body_name m.body_name m.title}}</button>
          <span class="dis-hit-sub">{{join
              (relDate m.date)
              m.time
              m.location
            }}</span>
        </div>
        <div class="dis-hit-meta">
          {{#if m.agenda_count}}
            <span class="dis-hit-stat"><Icon
                @name="docs-top"
                @size={{12}}
              />{{num m.agenda_count}}
              TOP</span>
          {{/if}}
          {{#if m.documents}}
            <span class="dis-hit-stat"><Icon
                @name="docs-files"
                @size={{12}}
              />{{num m.documents}}</span>
          {{/if}}
        </div>
      </li>
    {{/each}}
  </ol>
</template>;

export const PersonHits: TOC<{
  Args: { people: PersonHit[]; onOpen: (id: string) => void };
}> = <template>
  <ol class="dis-hits">
    {{#each @people key="person_id" as |p|}}
      <li class="dis-hit">
        <Avatar
          class="dis-hit-avatar"
          @personId={{p.person_id}}
          @name={{p.name}}
        />
        <div class="dis-hit-main">
          <button
            type="button"
            class="dis-hit-open"
            {{on "click" (fn @onOpen p.person_id)}}
          >{{p.name}}</button>
          {{#if p.memberships.length}}
            <span class="dis-hit-sub"><Icon
                @name="docs-body"
                @size={{12}}
              />{{num p.memberships.length}}
              Gremien</span>
          {{/if}}
        </div>
        <div class="dis-hit-meta">
          <PartyChip @code={{p.party_code}} @party={{p.party}} />
        </div>
      </li>
    {{/each}}
  </ol>
</template>;

export const CommitteeHits: TOC<{
  Args: { committees: CommitteeListItem[]; onOpen: (id: string) => void };
}> = <template>
  <ol class="dis-hits">
    {{#each @committees key="id" as |c|}}
      <li class="dis-hit">
        <span class="dis-hit-glyph"><Icon @name="suche-bodies" /></span>
        <div class="dis-hit-main">
          <button
            type="button"
            class="dis-hit-open"
            {{on "click" (fn @onOpen c.id)}}
          >{{c.name}}</button>
        </div>
        <div class="dis-hit-meta">
          {{#if c.members}}
            <span class="dis-hit-stat" title="Mitglieder"><Icon
                @name="users"
                @size={{12}}
              />{{num c.members}}</span>
          {{/if}}
          <span class="dis-hit-stat" title="Sitzungen"><Icon
              @name="calendar"
              @size={{12}}
            />{{num c.meetings}}</span>
          <span class="dis-hit-stat" title="Dokumente"><Icon
              @name="docs-files"
              @size={{12}}
            />{{num c.documents}}</span>
        </div>
      </li>
    {{/each}}
  </ol>
</template>;

export const SpeechHits: TOC<{
  Args: {
    speeches: SpeechHit[];
    q: string;
    personIds: Map<string, string>;
    onOpen: (meetingId: string) => void;
    onPerson: (personId: string) => void;
  };
}> = <template>
  <ol class="dis-hits">
    {{#each @speeches key="id" as |s|}}
      {{#let (personOf @personIds s.speaker) as |pid|}}
        <li class="dis-hit">
          {{#if pid}}
            <button
              type="button"
              class="dis-hit-person"
              aria-label={{s.speaker}}
              {{on "click" (fn @onPerson pid)}}
            ><Avatar
                class="dis-hit-avatar"
                @personId={{pid}}
                @name={{s.speaker}}
              /></button>
          {{else}}
            <Avatar class="dis-hit-avatar" @name={{s.speaker}} />
          {{/if}}
          <div class="dis-hit-main">
            <button
              type="button"
              class="dis-hit-open"
              {{on "click" (fn @onOpen s.meeting_id)}}
            >{{s.speaker}}</button>
            <span class="dis-hit-sub">{{join
                (top s.agenda_number)
                s.agenda_title
              }}</span>
            <Snippet @text={{highlight s.snippet @q}} />
          </div>
          <div class="dis-hit-meta">
            <span class="dis-hit-when">
              <time class="dis-hit-date" datetime={{s.meeting_date}}>{{dateDe
                  s.meeting_date
                }}</time>
              {{s.body_name}}
            </span>
            <span class="dis-tags">
              {{#if s.party_code}}
                <PartyChip @code={{s.party_code}} @party={{s.party}} />
              {{/if}}
              <span class="dis-hit-stat">{{num s.words}} Wörter</span>
            </span>
          </div>
        </li>
      {{/let}}
    {{/each}}
  </ol>
</template>;

export const VoteHits: TOC<{
  Args: { votes: VoteHit[]; q: string; onOpen: (meetingId: string) => void };
}> = <template>
  <ol class="dis-hits">
    {{#each @votes as |v i|}}
      {{#let (outcomeOf v) as |o|}}
        <li class="dis-hit">
          <span class="dis-hit-glyph dis-hit-glyph--{{o.key}}"><Icon
              @name={{iconOf o.key}}
            /></span>
          <div class="dis-hit-main">
            <button
              type="button"
              class="dis-hit-open"
              {{on "click" (fn @onOpen v.meeting_id)}}
            ><Snippet
                @text={{highlight
                  (if v.agenda_title v.agenda_title v.top_label)
                  @q
                }}
              /></button>
            <span class="dis-hit-sub">{{join
                (top v.agenda_number)
                (dateDe v.meeting_date)
                v.body_name
              }}</span>
          </div>
          <div class="dis-hit-meta">
            {{#if o}}
              <span class="dis-hit-outcome">
                <Tag
                  title={{if o.close "Knapp"}}
                  {{rough
                    "box"
                    color="--dis-highlight"
                    show=o.close
                    padding=2
                    delay=(stagger i)
                  }}
                >{{o.label}}</Tag>
              </span>
            {{/if}}
            <VoteBar
              class="dis-hit-votes"
              @ja={{v.ja}}
              @nein={{v.nein}}
              @enthaltung={{v.enthaltung}}
            />
          </div>
        </li>
      {{/let}}
    {{/each}}
  </ol>
</template>;
