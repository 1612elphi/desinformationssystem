import {
  getCommittees,
  getJSON,
  once,
  type CommitteeListItem,
  type Member,
} from 'ui/api';

export type PersonListItem = Omit<Member, 'role' | 'since'>;

// shared by the palette and Statistik
export const getPeople = once(() => getJSON<PersonListItem[]>('/api/people'));

export const getCommitteeList = once<CommitteeListItem[]>(getCommittees);
