import {
  getCommittees,
  getJSON,
  once,
  type CommitteeListItem,
  type Member,
} from 'ui/api';

export type PersonListItem = Omit<Member, 'role' | 'since'>;

export const getPeople = once(() => getJSON<PersonListItem[]>('/api/people'));

export const getCommitteeList = once<CommitteeListItem[]>(getCommittees);
