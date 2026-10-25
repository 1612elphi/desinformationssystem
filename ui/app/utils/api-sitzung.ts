import { getMeetings, type Meeting } from 'ui/api';

export interface Neighbours {
  prev?: Meeting;
  next?: Meeting;
}

// ponytail: two meetings of one body on the same day can show up as both prev and next
export async function getNeighbours(m: Meeting): Promise<Neighbours> {
  if (!m.body_id || !m.date) return {};
  const base = { committee: m.body_id, limit: 3 };
  const [before, after] = await Promise.all([
    getMeetings({ ...base, to: m.date }),
    getMeetings({ ...base, from: m.date, order: 'asc' }),
  ]);
  const other = (xs: Meeting[]) => xs.find((x) => x.id !== m.id);
  return { prev: other(before.meetings), next: other(after.meetings) };
}
