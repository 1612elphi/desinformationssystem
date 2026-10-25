import { getCommittees, type CommitteeListItem, type Member } from 'ui/api';

export type PersonListItem = Omit<Member, 'role' | 'since'>;

const once = <T>(load: () => Promise<T>) => {
  let p: Promise<T> | undefined;
  return () =>
    (p ??= load().catch((e: unknown) => {
      p = undefined;
      throw e;
    }));
};

// one request per session, shared by the palette and Statistik; a failure is retried
export const getPeople = once(async () => {
  const r = await fetch('/api/people');
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return r.json() as Promise<PersonListItem[]>;
});

export const getCommitteeList = once<CommitteeListItem[]>(getCommittees);
