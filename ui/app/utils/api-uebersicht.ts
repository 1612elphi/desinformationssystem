import {
  getAnalytics,
  getJSON,
  getMeeting,
  getMeetings,
  type Meeting,
} from 'ui/api';
import { contested, type ContestedVote } from 'ui/utils/api-dokumente';
import { addDays, iso } from 'ui/utils/calendar';
import {
  getLastCouncil,
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

// load: undefined is loading
export async function getLastSession(): Promise<{ last?: LastSession }> {
  const id = (await getLastCouncil())?.id;
  if (!id) return {};
  const [meeting, talk, ids] = await Promise.all([
    getMeeting(id) as Promise<SpeechMeeting>,
    getSpeechAnalytics(id).catch(() => undefined),
    getPersonIds().catch(() => new Map<string, string>()),
  ]);
  const speeches = Object.values(meeting.speech_counts ?? {}).reduce(
    (a, n) => a + n,
    0,
  );
  // chair opens every item
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

export async function getNextSessions(today: Date): Promise<NextSessions> {
  const { meetings } = await getMeetings({
    committee: COUNCIL,
    upcoming: true,
    from: iso(addDays(today, 1)),
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

const countRecent = async (since: string) =>
  (await getJSON<unknown[]>(`/api/recent?since=${since}&limit=${RECENT_CAP}`))
    .length;

export async function getPulse(today: Date): Promise<Pulse> {
  const week = iso(addDays(today, -7));
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
