import { getAnalytics, getMeeting, getMeetings, type Meeting } from 'ui/api';
import { contested, isoDay, type ContestedVote } from 'ui/utils/api-dokumente';
import {
  getPersonIds,
  getSpeechAnalytics,
  type SpeechAnalytics,
} from 'ui/utils/api-reden';

export const COUNCIL = 'Gemeinderat';

type SpeechMeeting = Meeting & { speech_counts?: Record<string, number> };

export interface Speaker {
  name: string;
  party: string;
  personId?: string;
  secs: number;
}

export interface LastSession {
  meeting: SpeechMeeting;
  contested: ContestedVote[];
  speeches: number;
  speakers: Speaker[];
}

const tomorrow = (today: Date) =>
  isoDay(new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1));

// wrapped so "no session" is a value; Load treats undefined as still loading
export async function getLastSession(
  today: Date,
): Promise<{ last?: LastSession }> {
  const { meetings } = await getMeetings({
    committee: COUNCIL,
    to: isoDay(today),
    limit: 1,
  });
  if (!meetings[0]) return {};
  const id = meetings[0].id;
  const [meeting, talk, ids] = await Promise.all([
    getMeeting(id) as Promise<SpeechMeeting>,
    getSpeechAnalytics(id).catch(() => undefined),
    getPersonIds().catch(() => new Map<string, string>()),
  ]);
  const speeches = Object.values(meeting.speech_counts ?? {}).reduce(
    (a, n) => a + n,
    0,
  );
  // OB is the chair: it opens every item and would top the list each time
  const speakers = (talk?.speakers ?? [])
    .filter((s) => s.party !== 'OB')
    .slice(0, 5)
    .map((s) => ({ ...s, personId: ids.get(s.name) }));
  return {
    last: {
      meeting,
      contested: contested(meeting, Infinity),
      speeches,
      speakers,
    },
  };
}

export interface NextSessions {
  next?: Meeting;
  later: Meeting[];
}

// the next session in full (for its agenda), the three after it from the list
export async function getNextSessions(today: Date): Promise<NextSessions> {
  const { meetings } = await getMeetings({
    committee: COUNCIL,
    upcoming: true,
    from: tomorrow(today),
    limit: 4,
  });
  const [first, ...later] = meetings;
  if (!first) return { later };
  return { next: { ...first, ...(await getMeeting(first.id)) }, later };
}

export interface Pulse {
  votes: number;
  contested: number;
  talk?: SpeechAnalytics;
  newDocs: number;
}

export const RECENT_CAP = 200;

async function countRecent(since: string) {
  const r = await fetch(`/api/recent?since=${since}&limit=${RECENT_CAP}`);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return ((await r.json()) as unknown[]).length;
}

export async function getPulse(today: Date): Promise<Pulse> {
  const week = isoDay(
    new Date(today.getFullYear(), today.getMonth(), today.getDate() - 7),
  );
  const [a, talk, newDocs] = await Promise.all([
    getAnalytics(),
    getSpeechAnalytics().catch(() => undefined),
    countRecent(week),
  ]);
  return {
    votes: a.totals.votes,
    contested: a.totals.contested,
    talk,
    newDocs,
  };
}
