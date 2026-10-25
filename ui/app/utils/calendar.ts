import type { Meeting } from 'ui/api';

export type CalMeeting = Meeting & { vote_count?: number };

export type Kind = 'rat' | 'aus' | 'ort' | 'other';

export const KINDS: { id: Kind; label: string }[] = [
  { id: 'rat', label: 'Gemeinderat' },
  { id: 'aus', label: 'Ausschüsse' },
  { id: 'ort', label: 'Ortschaftsräte' },
  { id: 'other', label: 'Beiräte, Sonstige' },
];

export function bodyKind(name?: string): Kind {
  const n = (name ?? '').toLowerCase();
  if (n.includes('ortschaftsrat')) return 'ort';
  if (n.includes('gemeinderat')) return 'rat';
  if (n.includes('ausschuss')) return 'aus';
  return 'other';
}

const pad = (n: number) => String(n).padStart(2, '0');

export const iso = (d: Date) =>
  `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

// local midnight; invalid: today
export function parseIso(s?: string): Date {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s ?? '');
  const d = m ? new Date(+m[1]!, +m[2]! - 1, +m[3]!) : new Date();
  if (isNaN(d.getTime())) return today();
  d.setHours(0, 0, 0, 0);
  return d;
}

export const today = () => parseIso(iso(new Date()));

export const dateDe = (s?: string) =>
  s ? s.slice(0, 10).split('-').reverse().join('.') : '';

export const addDays = (d: Date, n: number) =>
  new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);

// clamps to month end
export function addMonths(d: Date, n: number): Date {
  const last = new Date(d.getFullYear(), d.getMonth() + n + 1, 0).getDate();
  return new Date(
    d.getFullYear(),
    d.getMonth() + n,
    Math.min(d.getDate(), last),
  );
}

export const weekStart = (d: Date) => addDays(d, -((d.getDay() + 6) % 7));

export function monthWeeks(d: Date): Date[][] {
  const first = new Date(d.getFullYear(), d.getMonth(), 1);
  const last = new Date(d.getFullYear(), d.getMonth() + 1, 0);
  const weeks: Date[][] = [];
  for (let s = weekStart(first); s <= last; s = addDays(s, 7)) {
    weeks.push([0, 1, 2, 3, 4, 5, 6].map((i) => addDays(s, i)));
  }
  return weeks;
}

// iso 8601 week number
export function isoWeek(d: Date): number {
  const thu = addDays(d, 3 - ((d.getDay() + 6) % 7));
  const jan4 = new Date(thu.getFullYear(), 0, 4);
  return 1 + Math.round((thu.getTime() - weekStart(jan4).getTime()) / 6048e5);
}

export const isWeekend = (d: Date) => d.getDay() === 0 || d.getDay() === 6;

// "16.30 bis 18.30 Uhr" input
export function timeWindow(t?: string): { start: number; end: number } | null {
  const m = /(\d{1,2})(?:[.:](\d{2}))?\s*bis\s*(\d{1,2})(?:[.:](\d{2}))?/.exec(
    t ?? '',
  );
  if (!m) return null;
  const start = +m[1]! * 60 + +(m[2] ?? 0);
  const end = +m[3]! * 60 + +(m[4] ?? 0);
  return { start, end: Math.max(end, start + 30) };
}

export const clock = (min: number) =>
  `${Math.floor(min / 60)}${min % 60 ? `.${pad(min % 60)}` : ''}`;

export const startLabel = (t?: string) => {
  const w = timeWindow(t);
  return w ? clock(w.start) : '';
};

export function isLive(m: Meeting, now = new Date()): boolean {
  const w = timeWindow(m.time);
  if (!w || m.date !== iso(now)) return false;
  const min = now.getHours() * 60 + now.getMinutes();
  return min >= w.start && min <= w.end;
}

export function lanes<T>(
  items: T[],
  win: (x: T) => { start: number; end: number },
): { item: T; lane: number; lanes: number }[] {
  const sorted = [...items].sort((a, b) => win(a).start - win(b).start);
  const ends: number[] = [];
  const placed = sorted.map((item) => {
    const w = win(item);
    let lane = ends.findIndex((e) => e <= w.start);
    if (lane < 0) lane = ends.length;
    ends[lane] = w.end;
    return { item, lane };
  });
  return placed.map((p) => ({ ...p, lanes: ends.length }));
}

const fmt = (o: Intl.DateTimeFormatOptions) =>
  new Intl.DateTimeFormat('de-DE', o);

export const MONTH = fmt({ month: 'long', year: 'numeric' });
export const DAY_LONG = fmt({
  weekday: 'long',
  day: 'numeric',
  month: 'long',
  year: 'numeric',
});
export const DAY_SHORT = fmt({ weekday: 'short', day: 'numeric' });
export const RANGE = fmt({ day: 'numeric', month: 'long', year: 'numeric' });

export const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6].map((i) =>
  fmt({ weekday: 'short' })
    .format(new Date(2024, 0, 1 + i))
    .replace('.', ''),
);
