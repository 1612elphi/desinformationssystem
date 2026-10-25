import type { TOC } from '@ember/component/template-only';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import { htmlSafe } from '@ember/template';
import type { Member } from 'ui/api';
import Avatar from 'ui/components/avatar';
import { isChair, partyMark, roleTitle } from 'ui/utils/api-menschen';
import { SUBMITTERS } from 'ui/utils/meta';
import './gremium-members.css';

const markStyle = (m: Member) =>
  htmlSafe(`--dis-member-mark: ${partyMark(m.party_code)}`);
const partyLabel = (m: Member) =>
  m.party_code ? (SUBMITTERS[m.party_code]?.name ?? m.party_code) : m.party;

// Members as a dense grid of cards; each card opens the person sheet.
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
                <span class="dis-member-chair">Vorsitz</span>
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
