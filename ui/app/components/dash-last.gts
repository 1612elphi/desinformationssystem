import Component from '@glimmer/component';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import Avatar from 'ui/components/avatar';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import VoteBar from 'ui/components/vote-bar';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import type { ContestedVote } from 'ui/utils/api-dokumente';
import type { LastSession, Speaker } from 'ui/utils/api-uebersicht';
import { mins } from 'ui/utils/api-reden';
import { DAY_LONG, parseIso } from 'ui/utils/calendar';
import type Load from 'ui/utils/load';
import { eq, num } from 'ui/utils/meta';
import './dash-last.css';

const SHOWN_VOTES = 4;

const fmt = (f: Intl.DateTimeFormat) => (iso?: string) =>
  iso ? f.format(parseIso(iso)) : '';
const de = (o: Intl.DateTimeFormatOptions) =>
  fmt(new Intl.DateTimeFormat('de-DE', o));
const weekday = de({ weekday: 'short' });
const day = de({ day: 'numeric' });
const monthYear = de({ month: 'short', year: 'numeric' });
const longDate = fmt(DAY_LONG);

const passed = (v: ContestedVote) => (v.ja ?? 0) > (v.nein ?? 0);
const close = (v: ContestedVote) => Math.abs((v.ja ?? 0) - (v.nein ?? 0)) <= 3;
const stagger = (i: number) => 900 + i * 200;
const speakerLabel = (s: Speaker) => `${s.name}, ${mins(s.secs)}`;

export default class DashLast extends Component<{
  Args: { load: Load<{ last?: LastSession }> };
}> {
  @service declare nav: NavService;

  get last() {
    return this.args.load.value?.last;
  }

  get counts() {
    const l = this.last;
    if (!l) return [];
    return [
      {
        icon: 'dash-agenda',
        label: 'TOPs',
        value: num(l.meeting.agenda_items?.length ?? 0),
      },
      {
        icon: 'dash-vote',
        label: 'Abstimmungen',
        value: num(l.meeting.votes?.length ?? 0),
      },
      {
        icon: 'dash-contested',
        label: 'Strittig',
        value: num(l.contested.length),
        mark: l.contested.length > 0,
      },
      { icon: 'dash-speech', label: 'Reden', value: num(l.speeches) },
    ];
  }

  get votes() {
    return this.last?.contested.slice(0, SHOWN_VOTES) ?? [];
  }

  get moreVotes() {
    const n = (this.last?.contested.length ?? 0) - SHOWN_VOTES;
    return n > 0 ? `+${num(n)}` : '';
  }

  openMeeting = () => {
    const id = this.last?.meeting.id;
    if (id) this.nav.open({ meeting: id });
  };

  openPerson = (s: Speaker) => {
    if (s.personId) this.nav.open({ person: s.personId });
  };

  <template>
    <section class="dis-dlast" aria-labelledby="dis-dlast-title">
      <h2 id="dis-dlast-title" class="dis-dash-heading">
        <Icon @name="dash-last" @size={{18}} />
        Letzte Sitzung
      </h2>
      <LoadState @load={{@load}}>
        {{#if this.last}}
          {{#let this.last.meeting as |m|}}
            <div class="dis-dlast-top">
              <time
                class="dis-dlast-date"
                datetime={{m.date}}
                title={{longDate m.date}}
              >
                <span class="dis-dlast-wd">{{weekday m.date}}</span>
                <span class="dis-dlast-day">{{day m.date}}</span>
                <span class="dis-dlast-my">{{monthYear m.date}}</span>
              </time>
              <dl class="dis-dlast-counts">
                {{#each this.counts as |c|}}
                  <div>
                    <dt><Icon @name={{c.icon}} @size={{14}} />{{c.label}}</dt>
                    <dd>
                      <span
                        {{rough
                          "underline"
                          color="--dis-highlight"
                          strokeWidth=2
                          padding=1
                          show=(eq c.mark true)
                          delay=600
                        }}
                      >{{c.value}}</span>
                    </dd>
                  </div>
                {{/each}}
              </dl>
            </div>

            <div class="dis-dlast-part">
              <h3><Icon @name="dash-contested" @size={{14}} />Strittig
                {{#if this.moreVotes}}<span
                    class="dis-dlast-more"
                  >{{this.moreVotes}}</span>{{/if}}</h3>
              {{#if this.votes.length}}
                <ul class="dis-dlast-votes">
                  {{#each this.votes key="id" as |v i|}}
                    <li>
                      <button
                        type="button"
                        class="dis-dvote"
                        {{on "click" this.openMeeting}}
                      >
                        <span class="dis-dvote-head">
                          <span class="dis-dvote-top">{{v.top_label}}</span>
                          {{#if (close v)}}<span
                              class="dis-dvote-close"
                              {{rough
                                "box"
                                color="--dis-highlight"
                                padding=3
                                delay=(stagger i)
                              }}
                            >Knapp</span>{{/if}}
                          <span
                            class="dis-dvote-result
                              {{if (passed v) 'is-passed' 'is-failed'}}"
                          ><Icon
                              @name={{if
                                (passed v)
                                "docs-passed"
                                "docs-failed"
                              }}
                              @size={{14}}
                            />{{if (passed v) "Angenommen" "Abgelehnt"}}</span>
                        </span>
                        <span class="dis-dvote-title">{{if
                            v.title
                            v.title
                            v.top_label
                          }}</span>
                        <VoteBar
                          @ja={{v.ja}}
                          @nein={{v.nein}}
                          @enthaltung={{v.enthaltung}}
                        />
                      </button>
                    </li>
                  {{/each}}
                </ul>
              {{else}}
                <CanonEmptyRow @text="Keine Abstimmungen" />
              {{/if}}
            </div>

            {{#if this.last.speakers.length}}
              <div class="dis-dlast-part">
                <h3><Icon @name="dash-speakers" @size={{14}} />Meiste Redezeit</h3>
                <ol class="dis-dlast-speakers">
                  {{#each this.last.speakers as |s|}}
                    <li>
                      <button
                        type="button"
                        class="dis-dspeaker"
                        aria-label={{speakerLabel s}}
                        disabled={{unless s.personId true}}
                        {{on "click" (fn this.openPerson s)}}
                      >
                        <Avatar
                          class="dis-dspeaker-avatar"
                          @personId={{s.personId}}
                          @name={{s.name}}
                        />
                        <span class="dis-dspeaker-name">{{s.name}}</span>
                        <span class="dis-dspeaker-meta">
                          <PartyChip @code={{s.party}} />
                          {{mins s.secs}}
                        </span>
                      </button>
                    </li>
                  {{/each}}
                </ol>
              </div>
            {{/if}}

            <CanonButton
              class="dis-dlast-open"
              @kind="secondary"
              @icon={{component Icon name="dash-meeting"}}
              @text="Sitzung öffnen"
              @onClick={{this.openMeeting}}
            />
          {{/let}}
        {{else}}
          <CanonEmptyRow @text="Keine Sitzung" />
        {{/if}}
      </LoadState>
    </section>
  </template>
}
