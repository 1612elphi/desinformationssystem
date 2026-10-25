import type { Meeting, Vote } from 'ui/api';
import type { PersonListItem } from 'ui/utils/api-shell';

export interface SpeechHit {
  id: string;
  meeting_id: string;
  agenda_anchor?: string | null;
  agenda_number?: string | null;
  agenda_title?: string | null;
  top_label?: string | null;
  speaker: string;
  party?: string;
  party_code?: string;
  start_s?: number;
  snippet: string;
  words: number;
  meeting_date?: string;
  body_name?: string;
}

export interface VoteHit extends Omit<Vote, 'id'> {
  meeting_id: string;
  body_name?: string;
  body_id?: string;
  meeting_date?: string;
  agenda_number?: string | null;
  agenda_title?: string | null;
}

export interface Hits<T> {
  total: number;
  results: T[];
}

export type PersonHit = PersonListItem & {
  memberships?: { body_id: string; body_name: string; role?: string }[];
};

interface Page {
  q: string;
  limit: number;
  offset: number;
}

const query = (p: Record<string, string | number>) =>
  new URLSearchParams(
    Object.entries(p)
      .filter(([, v]) => v !== '')
      .map(([k, v]) => [k, String(v)]),
  ).toString();

async function hits<T>(path: string, p: Page): Promise<Hits<T>> {
  const r = await fetch(`${path}?${query({ ...p })}`);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return r.json() as Promise<Hits<T>>;
}

export const searchSpeeches = (p: Page) =>
  hits<SpeechHit>('/api/speeches/search', p);

export const searchVotes = (p: Page) => hits<VoteHit>('/api/votes', p);

// with no query, the newest past meetings rather than placeholder dates years ahead
export async function searchMeetings(p: Page): Promise<Hits<Meeting>> {
  const to = p.q ? '' : new Date().toISOString().slice(0, 10);
  const r = await fetch(`/api/meetings?${query({ ...p, to })}`);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  const results = (await r.json()) as Meeting[];
  const total = Number(r.headers.get('X-Total-Count') || results.length);
  return { total, results };
}

export const fold = (s: string) =>
  s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();

// every whitespace-separated term occurs in one of the fields
export function matches(q: string, fields: (string | null | undefined)[]) {
  const hay = fold(fields.filter(Boolean).join(' '));
  return fold(q)
    .split(/\s+/)
    .filter(Boolean)
    .every((t) => hay.includes(t));
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// wraps each query term in the \x01…\x02 delimiters the Snippet component highlights
export function highlight(text: string | null | undefined, q: string) {
  const terms = q.split(/\s+/).filter((t) => t.length > 1);
  if (!text || !terms.length) return text ?? '';
  const re = new RegExp(`(${terms.map(escape).join('|')})`, 'gi');
  return text.replace(re, '\u0001$1\u0002');
}
