import {
  getLive,
  getMeetings,
  search,
  type Doc,
  type Meeting,
  type SearchParams,
  type SearchResult,
  type Vote,
} from 'ui/api';

export interface FacetCount {
  value: string;
  n: number;
}

export interface FacetCounts {
  doc_type?: FacetCount[];
  category?: FacetCount[];
}

export interface FacetedResult extends SearchResult {
  facet_counts?: FacetCounts;
}

// search() sends every set key, so the newer category and facets params pass through.
export const searchFaceted = (
  p: SearchParams & { category?: string; facets?: boolean },
) => search(p) as Promise<FacetedResult>;

export interface RecentDoc extends Doc {
  downloaded_at?: string;
}

// The newest documents, one per agenda item: attachments arrive in batches.
export async function getRecent(limit: number): Promise<RecentDoc[]> {
  const r = await fetch(`/api/recent?limit=${limit * 4}`);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  const seen = new Set<string>();
  return ((await r.json()) as RecentDoc[])
    .filter((d) => {
      const k = `${d.body_name}|${d.agenda_title || d.label}`;
      return !seen.has(k) && !!seen.add(k);
    })
    .slice(0, limit);
}

// Local calendar date as YYYY-MM-DD (sv-SE formats dates in ISO order).
export const isoDay = (d: Date) => d.toLocaleDateString('sv-SE');

const addDays = (d: Date, n: number) =>
  new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);

// Monday to Sunday of the week that holds `today`.
export function weekDays(today: Date): string[] {
  const monday = addDays(today, -((today.getDay() + 6) % 7));
  return Array.from({ length: 7 }, (_, i) => isoDay(addDays(monday, i)));
}

const rtf = new Intl.RelativeTimeFormat('de-DE', { numeric: 'auto' });

// "heute", "vor 3 Tagen", "in 2 Wochen" for a YYYY-MM-DD date.
export function relDate(iso?: string, today = new Date()): string {
  if (!iso) return '';
  const [y, m, d] = iso.slice(0, 10).split('-').map(Number);
  if (!y || !m || !d) return '';
  const start = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate(),
  );
  const days = Math.round(
    (new Date(y, m - 1, d).getTime() - start.getTime()) / 864e5,
  );
  const abs = Math.abs(days);
  if (abs < 7) return rtf.format(days, 'day');
  if (abs < 30) return rtf.format(Math.trunc(days / 7), 'week');
  if (abs < 365) return rtf.format(Math.trunc(days / 30), 'month');
  return rtf.format(Math.trunc(days / 365), 'year');
}

export interface ContestedVote extends Vote {
  title?: string;
  meeting_id: string;
}

// Votes with both Ja and Nein, closest margin first.
export function contested(m: Meeting, limit: number): ContestedVote[] {
  const titles = new Map(
    (m.agenda_items ?? []).map((a) => [a.anchor, a.title]),
  );
  const margin = (v: Vote) => {
    const total = (v.ja ?? 0) + (v.nein ?? 0) + (v.enthaltung ?? 0);
    return Math.abs((v.ja ?? 0) - (v.nein ?? 0)) / (total || 1);
  };
  return (m.votes ?? [])
    .filter((v) => (v.ja ?? 0) > 0 && (v.nein ?? 0) > 0)
    .sort((a, b) => margin(a) - margin(b))
    .slice(0, limit)
    .map((v) => ({
      ...v,
      meeting_id: m.id,
      title: v.agenda_anchor ? titles.get(v.agenda_anchor) : undefined,
    }));
}

export interface Week {
  days: string[];
  meetings: Meeting[];
  live: string[];
}

export async function getWeek(today: Date): Promise<Week> {
  const days = weekDays(today);
  const [{ meetings }, live] = await Promise.all([
    getMeetings({ from: days[0], to: days[6], limit: 200 }),
    getLive().catch(() => ({ meetings: [] as Meeting[] })),
  ]);
  const key = (x: Meeting) => `${x.date ?? ''} ${x.time ?? ''}`;
  return {
    days,
    meetings: meetings.sort((a, b) => key(a).localeCompare(key(b))),
    live: live.meetings.map((x) => x.id),
  };
}
