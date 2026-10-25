import { getJSON, getMeetings, once, type Meeting } from 'ui/api';
import { getPeople } from 'ui/utils/api-shell';
import { iso } from 'ui/utils/calendar';

export interface Speech {
  id: string;
  agenda_anchor?: string | null;
  speaker?: string | null;
  party?: string | null;
  party_code?: string | null;
  video_id?: string;
  start_s: number;
  end_s: number;
  text: string;
}

export const getSpeeches = (id: string) =>
  getJSON<Speech[]>(`/api/meeting/${id}/speeches`);

export interface SpeechParty {
  // party code, or OB for the chair
  code: string;
  seats: number;
  seat_share: number | null;
  time_share: number | null;
  secs: number;
  words: number;
  speeches: number;
  speakers: number;
  // -1..1, word-weighted
  sentiment: number | null;
  tone: Record<string, number> | null;
  flesch: number | null;
  sentence_len: number | null;
}

export interface SaidVotedRow {
  meeting_id: string;
  anchor: string;
  speaker: string;
  party: string;
  stance: string;
  p: number;
  vote: string | null;
  match: boolean | null;
  joint: boolean;
}

export interface SpeechTop {
  meeting_id: string;
  anchor: string;
  number: string;
  title: string;
  date: string;
  secs: number;
  speeches: number;
  parties: number;
  ja: number | null;
  nein: number | null;
  enthaltung: number | null;
  dissent: number | null;
}

export interface SpeechAnalytics {
  meetings: { id: string; date: string; body_name: string }[];
  totals: {
    speeches: number;
    secs: number;
    words: number;
    speakers: number;
    meetings: number;
    judged: number;
  };
  parties: SpeechParty[];
  speakers: {
    name: string;
    party: string;
    secs: number;
    words: number;
    speeches: number;
  }[];
  said_voted: { n: number; match: number; rows: SaidVotedRow[] };
  matrix: { from: string; to: string; n: number; attitude: number | null }[];
  words: {
    overall: { word: string; n: number }[];
    distinctive: Record<string, { word: string; z: number; n: number }[]>;
  };
  tops: SpeechTop[];
}

const getAllSpeechAnalytics = once(() =>
  getJSON<SpeechAnalytics>('/api/speeches/analytics'),
);

export const getSpeechAnalytics = (meeting?: string) =>
  meeting
    ? getJSON<SpeechAnalytics>(
        `/api/speeches/analytics?meeting=${encodeURIComponent(meeting)}`,
      )
    : getAllSpeechAnalytics();

export const getLastCouncil = once(async (): Promise<Meeting | undefined> => {
  const r = await getMeetings({
    committee: 'Gemeinderat',
    to: iso(new Date()),
    limit: 1,
  });
  return r.meetings[0];
});

// speeches and analytics carry the speaker's name only; it matches /api/people exactly
export async function getPersonIds(): Promise<Map<string, string>> {
  const people = await getPeople();
  return new Map(people.map((p) => [p.name, p.person_id]));
}

export const clock = (sec: number) => {
  const s = Math.floor(sec);
  const mm = String(Math.floor((s % 3600) / 60)).padStart(2, '0');
  const ss = String(s % 60).padStart(2, '0');
  return `${Math.floor(s / 3600)}:${mm}:${ss}`;
};

export const mins = (s: number) => `${Math.round(s / 60)} min`;
