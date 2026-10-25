import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import type { Member } from 'ui/api';
import Avatar from 'ui/components/avatar';
import Icon from 'ui/components/icon';
import rough from 'ui/modifiers/rough';
import {
  isChair,
  partyMark,
  partyName,
  roleTitle,
} from 'ui/utils/api-menschen';
import './gremium-members.css';

const markStyle = (m: Member) =>
  htmlSafe(`--dis-member-mark: ${partyMark(m.party_code)}`);
const partyLabel = (m: Member) =>
  m.party_code ? partyName(m.party_code) : m.party;

const GremiumMembers: TOC<{
  Element: HTMLUListElement;
  Args: { members: Member[]; onOpen: (personId: string) => void };
}> = <template>
  <ul class="dis-members" ...attributes>
    {{#each @members key="person_id" as |m|}}
      <li>
        <button
          type="button"
          class="dis-member"
          style={{markStyle m}}
          {{on "click" (fn @onOpen m.person_id)}}
        >
          <Avatar
            class="dis-member-avatar"
            @personId={{m.person_id}}
            @name={{m.name}}
          />
          <span class="dis-member-text">
            <span class="dis-member-name">{{m.name}}</span>
            <span class="dis-member-meta">
              <span title={{partyLabel m}}>{{if
                  m.party_code
                  m.party_code
                  m.party
                }}</span>
              {{#if (isChair m.role)}}
                <span
                  class="dis-member-chair"
                  {{rough "box" padding=3 strokeWidth=1.2 delay=900}}
                ><Icon @name="crown" @size={{12}} />Vorsitz</span>
              {{/if}}
              <span class="dis-member-role">{{roleTitle m.role}}</span>
            </span>
          </span>
        </button>
      </li>
    {{/each}}
  </ul>
</template>;

export default GremiumMembers;
