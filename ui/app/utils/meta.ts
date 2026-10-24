import type { Vote } from 'ui/api';

export const RIS_BASE =
  'https://sitzungskalender.karlsruhe.de/db/ratsinformation';
export const RIS_START = `${RIS_BASE}/start`;

export const PUBLIC_OPTS = [
  { id: 'all', label: 'Öffentlichkeit: alle' },
  { id: 'public', label: 'Nur öffentlich' },
  { id: 'nonpublic', label: 'Nur nicht öffentlich' },
];

// Submitter code: chip mark colour (party colour where one is established) and
// full name (chip tooltip, filter list).
export const SUBMITTERS: Record<string, { mark?: string; name: string }> = {
  SVK: { name: 'Stadtverwaltung Karlsruhe' },
  CDU: {
    mark: 'var(--canon-foreground)',
    name: 'Christlich Demokratische Union',
  },
  B90: { mark: '#46962b', name: 'Bündnis 90/Die Grünen' },
  SPD: { mark: '#e3000f', name: 'Sozialdemokratische Partei Deutschlands' },
  LIN: { mark: '#be3075', name: 'Die Linke' },
  FDP: { mark: '#ffed00', name: 'Freie Demokratische Partei' },
  AFD: { mark: '#009ee0', name: 'Faschistische Nazi-Hurensöhne' },
  FWV: { mark: '#f39200', name: 'Freie Wähler' },
  KAL: { name: 'Karlsruher Liste' },
  VOL: { mark: '#502379', name: 'Volt' },
  PAR: { name: 'Die PARTEI' },
  HEI: { name: 'Die Heimat' },
};

// Display order for the originator filter (SVK first, AfD near the end).
export const SUBMITTER_ORDER = [
  'SVK',
  'CDU',
  'B90',
  'SPD',
  'LIN',
  'FDP',
  'FWV',
  'KAL',
  'VOL',
  'PAR',
  'AFD',
  'HEI',
];

export const VOTE_LABEL: Record<string, string> = {
  ja: 'Ja',
  nein: 'Nein',
  enthaltung: 'Enthaltung',
  abwesend: 'Abwesend',
};

export function publicLabel(p?: number | null): { text: string } {
  if (p === 1) return { text: 'öffentlich' };
  if (p === 0) return { text: 'nicht öffentlich' };
  return { text: 'gemischt' };
}

export function pct(x: number | null | undefined): string {
  return x == null ? '—' : `${Math.round(x * 100)} %`;
}

export function num(n: number | null | undefined): string {
  return (n ?? 0).toLocaleString('de-DE');
}

export function hasTally(v?: Vote): boolean {
  return !!v && (v.ja != null || v.nein != null || v.enthaltung != null);
}

export function tally(v: Vote): string {
  return `${v.ja ?? 0}·${v.nein ?? 0}·${v.enthaltung ?? 0}`;
}

export function scrapeTime(s?: string | null): string {
  return s ? s.replace('T', ' ').slice(0, 16) : '—';
}

export const eq = (a: unknown, b: unknown): boolean => a === b;
export const or = <T>(...xs: T[]): T | undefined => xs.find(Boolean);

export function inputValue(fn: (v: string) => void) {
  return (e: Event) =>
    fn((e.target as HTMLInputElement | HTMLSelectElement).value);
}
