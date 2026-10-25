import type { Meeting } from 'ui/api';

async function list(p: Record<string, string>): Promise<Meeting[]> {
  const r = await fetch(`/api/meetings?${new URLSearchParams(p).toString()}`);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return r.json() as Promise<Meeting[]>;
}

export interface Neighbours {
  prev?: Meeting;
  next?: Meeting;
}

// The meetings of the same body right before and after this one.
// ponytail: two meetings of one body on the same day can show up as both prev and next
export async function getNeighbours(m: Meeting): Promise<Neighbours> {
  if (!m.body_id || !m.date) return {};
  const base = { committee: m.body_id, limit: '3' };
  const [before, after] = await Promise.all([
    list({ ...base, to: m.date }),
    list({ ...base, from: m.date, order: 'asc' }),
  ]);
  const other = (xs: Meeting[]) => xs.find((x) => x.id !== m.id);
  return { prev: other(before), next: other(after) };
}
