import Component from '@glimmer/component';
import { service } from '@ember/service';
import { fn } from '@ember/helper';
import { on } from '@ember/modifier';
import CanonButton from 'delphicanon/components/canon-button';
import CanonEmptyRow from 'delphicanon/components/canon-empty-row';
import type { Meeting } from 'ui/api';
import Icon from 'ui/components/icon';
import { LoadState } from 'ui/components/loading';
import rough from 'ui/modifiers/rough';
import type NavService from 'ui/services/nav';
import { relDate } from 'ui/utils/api-dokumente';
import type { NextSessions } from 'ui/utils/api-uebersicht';
import type Load from 'ui/utils/load';
import { num } from 'ui/utils/meta';
import './dash-next.css';

const PREVIEW = 4;

const parse = (iso?: string) => {
  const [y, m, d] = (iso ?? '').split('-').map(Number);
  return new Date(y ?? 0, (m ?? 1) - 1, d ?? 1);
};
const longDate = new Intl.DateTimeFormat('de-DE', {
  weekday: 'long',
  day: 'numeric',
  month: 'long',
  year: 'numeric',
});
const shortDate = new Intl.DateTimeFormat('de-DE', {
  weekday: 'short',
  day: 'numeric',
  month: 'short',
});
const rtf = new Intl.RelativeTimeFormat('de-DE', { numeric: 'auto' });

const long = (iso?: string) => (iso ? longDate.format(parse(iso)) : '');
const short = (iso?: string) => (iso ? shortDate.format(parse(iso)) : '');
const topNum = (n?: string) => (n ? `TOP ${n}` : 'TOP');

export default class DashNext extends Component<{
  Args: { load: Load<NextSessions> };
}> {
  @service declare nav: NavService;

  today = new Date();

  get next() {
    return this.args.load.value?.next;
  }

  get days() {
    const d = this.next?.date;
    if (!d) return Infinity;
    const start = new Date(
      this.today.getFullYear(),
      this.today.getMonth(),
      this.today.getDate(),
    );
    return Math.round((parse(d).getTime() - start.getTime()) / 864e5);
  }

  get countdown() {
    return Number.isFinite(this.days) ? rtf.format(this.days, 'day') : '';
  }

  get soon() {
    return this.days <= 7;
  }

  get preview() {
    return (this.next?.agenda_items ?? []).slice(0, PREVIEW);
  }

  get moreAgenda() {
    const n = (this.next?.agenda_items?.length ?? 0) - PREVIEW;
    return n > 0 ? `+${num(n)}` : '';
  }

  get tops() {
    return num(this.next?.agenda_items?.length ?? this.next?.agenda_count ?? 0);
  }

  relative = (iso?: string) => relDate(iso, this.today);

  open = (m?: Meeting) => {
    if (m) this.nav.open({ meeting: m.id });
  };

  <template>
    <section class="dis-dnext" aria-labelledby="dis-dnext-title">
      <h2 id="dis-dnext-title" class="dis-dash-heading">
        <Icon @name="dash-next" @size={{18}} />
        Nächste Sitzung
      </h2>
      <LoadState @load={{@load}}>
        {{#if this.next}}
          {{#let this.next as |m|}}
            <div class="dis-dnext-card">
              <p class="dis-dnext-count">
                <span
                  {{rough
                    "circle"
                    color="--canon-primary"
                    padding=8
                    strokeWidth=2
                    show=this.soon
                    delay=400
                  }}
                >{{this.countdown}}</span>
              </p>
              <ul class="dis-dnext-facts">
                <li><Icon @name="calendar" @size={{14}} /><time
                    datetime={{m.date}}
                  >{{long m.date}}</time></li>
                {{#if m.time}}
                  <li><Icon @name="time" @size={{14}} />{{m.time}}</li>
                {{/if}}
                {{#if m.location}}
                  <li><Icon
                      @name="dash-place"
                      @size={{14}}
                    />{{m.location}}</li>
                {{/if}}
              </ul>
              <dl class="dis-dnext-counts">
                <div>
                  <dt><Icon @name="dash-agenda" @size={{14}} />TOPs</dt>
                  <dd>{{this.tops}}</dd>
                </div>
                <div>
                  <dt><Icon @name="dash-docs" @size={{14}} />Dokumente</dt>
                  <dd>{{num m.documents}}</dd>
                </div>
              </dl>
            </div>

            <div class="dis-dnext-part">
              <h3><Icon @name="dash-agenda" @size={{14}} />Tagesordnung
                {{#if this.moreAgenda}}<span
                    class="dis-dnext-more"
                  >{{this.moreAgenda}}</span>{{/if}}</h3>
              {{#if this.preview.length}}
                <ol class="dis-dnext-agenda">
                  {{#each this.preview key="anchor" as |a|}}
                    <li>
                      <span class="dis-dnext-topnum">{{topNum a.number}}</span>
                      <span class="dis-dnext-toptitle">{{a.title}}</span>
                    </li>
                  {{/each}}
                </ol>
              {{else}}
                <CanonEmptyRow @text="Noch keine Tagesordnung" />
              {{/if}}
            </div>

            <CanonButton
              class="dis-dnext-open"
              @kind="secondary"
              @icon={{component Icon name="dash-meeting"}}
              @text="Sitzung öffnen"
              @onClick={{fn this.open m}}
            />
          {{/let}}
        {{else}}
          <CanonEmptyRow @text="Keine Sitzung" />
        {{/if}}

        {{#if @load.value.later.length}}
          <div class="dis-dnext-part">
            <h3><Icon @name="dash-later" @size={{14}} />Danach</h3>
            <ol class="dis-dnext-later">
              {{#each @load.value.later key="id" as |l|}}
                <li>
                  <button
                    type="button"
                    class="dis-dnext-date"
                    {{on "click" (fn this.open l)}}
                  >
                    <time datetime={{l.date}}>{{short l.date}}</time>
                    <span class="dis-dnext-rel">{{this.relative l.date}}</span>
                  </button>
                </li>
              {{/each}}
            </ol>
          </div>
        {{/if}}
      </LoadState>
    </section>
  </template>
}
