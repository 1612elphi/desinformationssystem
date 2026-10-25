import type {
  Committee,
  CommitteeListItem,
  Doc,
  Member,
  Person,
  PersonVote,
} from 'ui/api';
import { dateDe, parseIso, today } from 'ui/utils/calendar';
import { SEAT_ORDER, SUBMITTERS } from 'ui/utils/meta';

export interface PartyVote extends PersonVote {
  ja?: number | null;
  nein?: number | null;
  enthaltung?: number | null;
  party_code?: string;
  party_line?: string;
  party_tally?: Record<string, number>;
}

export interface PersonDetail extends Person {
  votes: PartyVote[];
}

export interface CommitteeDetail extends Committee {
  recent_documents?: Doc[];
}

const CAST = ['ja', 'nein', 'enthaltung'];

export const againstParty = (v: PartyVote) =>
  !!v.party_line && CAST.includes(v.vote) && v.vote !== v.party_line;

export const partyMark = (code?: string | null) =>
  (code && SUBMITTERS[code]?.mark) || 'var(--canon-muted-foreground)';

export const partyName = (code: string) => SUBMITTERS[code]?.name ?? code;

export function initials(name = ''): string {
  const core = name
    .split(/\s+/)
    .filter((t) => t && !t.endsWith('.') && /^\p{Lu}/u.test(t));
  const pick = core.length > 1 ? [core[0]!, core.at(-1)!] : core;
  return pick.map((t) => t[0]).join('');
}

const surname = (name: string) =>
  name
    .split(/\s+/)
    .filter((t) => !t.endsWith('.'))
    .at(-1) ?? name;

const seatRank = (code?: string) => {
  const i = SEAT_ORDER.indexOf(code ?? '');
  return i < 0 ? SEAT_ORDER.length : i;
};

export const byChamber = (members: Member[]) =>
  [...members].sort(
    (a, b) =>
      seatRank(a.party_code) - seatRank(b.party_code) ||
      surname(a.name).localeCompare(surname(b.name), 'de'),
  );

export type CommitteeGroup =
  'Gemeinderat' | 'Ausschüsse' | 'Ortschaftsräte' | 'Weitere';

export const GROUP_ORDER: CommitteeGroup[] = [
  'Gemeinderat',
  'Ausschüsse',
  'Ortschaftsräte',
  'Weitere',
];

export function committeeGroup(c: CommitteeListItem): CommitteeGroup {
  if (c.name.startsWith('Gemeinderat')) return 'Gemeinderat';
  if (c.name.startsWith('Ortschaftsrat')) return 'Ortschaftsräte';
  if (/ausschuss/i.test(c.name)) return 'Ausschüsse';
  return 'Weitere';
}

export const roleTitle = (role?: string) =>
  (role ?? '')
    .replace(
      /\s*(Ordentliches Mitglied|Stellvertretendes Mitglied|Vorsitz)$/,
      '',
    )
    .trim();

export const isChair = (role?: string) => /Vorsitz/.test(role ?? '');

const WEEKDAY_DATE = new Intl.DateTimeFormat('de-DE', {
  weekday: 'short',
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
});

export function deDate(s?: string, weekday = false) {
  if (!s) return '—';
  return weekday ? WEEKDAY_DATE.format(parseIso(s)) : dateDe(s);
}

export function daysUntil(iso?: string): string {
  if (!iso) return '';
  const ms = parseIso(iso).getTime() - today().getTime();
  const d = Math.round(ms / 86400000);
  return d === 0 ? 'heute' : d === 1 ? 'morgen' : `in ${d} Tagen`;
}
