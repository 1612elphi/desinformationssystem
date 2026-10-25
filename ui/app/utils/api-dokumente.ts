import {
  getJSON,
  getLive,
  getMeetings,
  search,
  type Doc,
  type Meeting,
  type SearchParams,
  type SearchResult,
  type Vote,
} from 'ui/api';
import { addDays, iso, weekStart } from 'ui/utils/calendar';

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

// search() forwards every key
export const searchFaceted = (
  p: SearchParams & { category?: string; facets?: boolean },
) => search(p) as Promise<FacetedResult>;

export interface RecentDoc extends Doc {
  downloaded_at?: string;
}

// attachments arrive in batches
export async function getRecent(limit: number): Promise<RecentDoc[]> {
  const docs = await getJSON<RecentDoc[]>(`/api/recent?limit=${limit * 4}`);
  const seen = new Set<string>();
  return docs
    .filter((d) => {
      const k = `${d.body_name}|${d.agenda_title || d.label}`;
      return !seen.has(k) && !!seen.add(k);
    })
    .slice(0, limit);
}

export function weekDays(today: Date): string[] {
  const monday = weekStart(today);
  return Array.from({ length: 7 }, (_, i) => iso(addDays(monday, i)));
}

const rtf = new Intl.RelativeTimeFormat('de-DE', { numeric: 'auto' });

export function relDate(s?: string, today = new Date()): string {
  if (!s) return '';
  const [y, m, d] = s.slice(0, 10).split('-').map(Number);
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
