import Component from '@glimmer/component';
import CanonBar from 'delphicanon/components/canon-bar';
import type { Stats } from 'ui/api';
import Icon from 'ui/components/icon';
import { LoadError } from 'ui/components/loading';
import PartyChip from 'ui/components/party-chip';
import rough from 'ui/modifiers/rough';
import { getPulse, RECENT_CAP } from 'ui/utils/api-uebersicht';
import Load from 'ui/utils/load';
import { eq, num, pct, scrapeTime } from 'ui/utils/meta';
import './dash-stats.css';

interface Tile {
  icon: string;
  label: string;
  value: string;
  level?: number;
  party?: string;
  mark?: boolean;
}

const share = (a: number, b: number) => (b ? a / b : 0);
const stagger = (i: number) => 500 + i * 120;

interface Signature {
  Args: { stats?: Stats; error?: string };
}

export default class DashStats extends Component<Signature> {
  pulse = new Load(getPulse(new Date()));

  get tiles(): Tile[] {
    const s = this.args.stats;
    const p = this.pulse.value;
    const out: Tile[] = [];
    if (s)
      out.push(
        { icon: 'dash-docs', label: 'Dokumente', value: num(s.documents) },
        { icon: 'dash-meeting', label: 'Sitzungen', value: num(s.meetings) },
        {
          icon: 'dash-text',
          label: 'Volltext',
          value: num(s.with_text),
          level: share(s.with_text, s.documents),
        },
        {
          icon: 'dash-analysed',
          label: 'Analysiert',
          value: num(s.enriched),
          level: share(s.enriched, s.documents),
        },
        {
          icon: 'dash-scrape',
          label: 'Letzter Abruf',
          value: scrapeTime(s.last_scrape),
        },
      );
    if (p) {
      const top = (p.talk?.parties ?? [])
        .filter((x) => x.code !== 'OB')
        .sort((a, b) => b.secs - a.secs)[0];
      out.push(
        {
          icon: 'dash-new',
          label: 'Neu · 7 Tage',
          value:
            p.newDocs >= RECENT_CAP ? `${num(RECENT_CAP)}+` : num(p.newDocs),
          mark: p.newDocs > 0,
        },
        { icon: 'dash-vote', label: 'Abstimmungen', value: num(p.votes) },
        {
          icon: 'dash-contested',
          label: 'Strittig',
          value: pct(share(p.contested, p.votes)),
          level: share(p.contested, p.votes),
        },
      );
      if (p.talk)
        out.push({
          icon: 'dash-speech',
          label: 'Reden',
          value: num(p.talk.totals.speeches),
        });
      if (top)
        out.push({
          icon: 'dash-party',
          label: 'Meiste Redezeit',
          value: pct(top.time_share),
          party: top.code,
        });
    }
    return out;
  }

  <template>
    <section class="dis-dstats" aria-labelledby="dis-dstats-title">
      <h2 id="dis-dstats-title" class="dis-dash-heading">
        <Icon @name="dash-stats" @size={{18}} />
        Zahlen
      </h2>
      {{#if @error}}<LoadError @error={{@error}} />{{/if}}
      {{#if this.pulse.error}}<LoadError @error={{this.pulse.error}} />{{/if}}
      <dl class="dis-dstats-tiles">
        {{#each this.tiles as |t i|}}
          <div class="dis-dstat">
            <dt><Icon @name={{t.icon}} @size={{14}} />{{t.label}}</dt>
            <dd>
              {{#if t.party}}<PartyChip @code={{t.party}} />{{/if}}
              <span
                {{rough
                  "underline"
                  color="--dis-highlight"
                  strokeWidth=2
                  padding=1
                  show=(eq t.mark true)
                  delay=(stagger i)
                }}
              >{{t.value}}</span>
            </dd>
            {{#if t.level}}
              <CanonBar class="dis-dstat-bar" @share={{t.level}} />
            {{/if}}
          </div>
        {{/each}}
      </dl>
    </section>
  </template>
}
