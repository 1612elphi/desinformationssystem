export interface Doc {
  id: string;
  url: string;
  label: string;
  doc_type: string;
  filename: string;
  meeting_id: string;
  body_name?: string;
  meeting_title?: string;
  meeting_date?: string;
  agenda_anchor?: string;
  agenda_number?: string;
  agenda_title?: string;
  category?: string;
  summary_de?: string;
  summary_en?: string;
  topics?: string[];
  submitters?: string[];
  entities?: { people?: string[]; orgs?: string[]; locations?: string[] };
  text_status?: string;
  enrich_status?: string;
  location?: string;
  fulltext?: string;
  snippet?: string; // FTS match context; / delimit highlights
  vorlage?: string;
  vorlagen?: { vorlage: string; own: number }[];
  versions?: FileVersion[];
}

export interface FileVersion {
  id: number;
  sha256?: string;
  size?: number;
  remote_modified?: string;
  downloaded_at?: string;
  superseded_at?: string;
}

export interface SearchResult {
  total: number;
  results: Doc[];
}

export interface Facets {
  committees: { id: string; name: string }[];
  doc_types: string[];
  submitters: string[];
}

export interface Stats {
  bodies: number;
  meetings: number;
  documents: number;
  with_text: number;
  enriched: number;
  last_scrape: string | null;
  last_scrape_status: string | null;
}

export interface SearchParams {
  q?: string;
  committee?: string;
  type?: string;
  from?: string;
  to?: string;
  public?: string;
  topic?: string;
  submitter?: string;
  limit?: number;
  offset?: number;
}

async function getJSON<T>(url: string): Promise<T> {
  const r = await fetch(url);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return r.json() as Promise<T>;
}

export function search(p: SearchParams): Promise<SearchResult> {
  const qs = new URLSearchParams();
  Object.entries(p).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== "") qs.set(k, String(v));
  });
  return getJSON<SearchResult>(`/api/search?${qs.toString()}`);
}

export interface Meeting {
  id: string;
  body_name?: string;
  body_id?: string;
  title?: string;
  date?: string;
  time?: string;
  location?: string;
  public?: number | null;
  url?: string;
  documents?: number;
  agenda_count?: number;
  agenda_items?: { anchor: string; number?: string; title?: string; public?: number | null }[];
  files?: Doc[];
  votes?: Vote[];
  _refresh?: { ok: boolean; error?: string; counts?: Record<string, number> };
}

export interface Vote {
  id: string;
  agenda_anchor?: string;
  top_label?: string;
  result_text?: string;
  ja?: number | null;
  nein?: number | null;
  enthaltung?: number | null;
  members?: { name: string; vote: string }[];
  source?: string;
  image_url?: string;
}

export interface Member {
  person_id: string;
  name: string;
  party?: string;
  party_code?: string;
  role?: string;
  since?: string;
}

export interface CommitteeListItem {
  id: string;
  name: string;
  members: number;
  meetings: number;
  documents: number;
}

export interface Committee {
  id: string;
  name: string;
  members: Member[];
  composition: { code: string; n: number }[];
  meetings: Meeting[];
  document_count: number;
}

export const getFacets = () => getJSON<Facets>("/api/facets");
export const getStats = () => getJSON<Stats>("/api/stats");
export const getDocument = (id: string) => getJSON<Doc>(`/api/document/${id}`);
export const fileUrl = (id: string, download = false) =>
  `/api/file/${id}${download ? "?download=true" : ""}`;

export async function getMeetings(p: {
  committee?: string;
  upcoming?: boolean;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}): Promise<{ meetings: Meeting[]; total: number }> {
  const qs = new URLSearchParams();
  if (p.committee) qs.set("committee", p.committee);
  if (p.upcoming) qs.set("upcoming", "true");
  if (p.from) qs.set("from", p.from);
  if (p.to) qs.set("to", p.to);
  qs.set("limit", String(p.limit ?? 200));
  if (p.offset) qs.set("offset", String(p.offset));
  const r = await fetch(`/api/meetings?${qs.toString()}`);
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  const meetings = (await r.json()) as Meeting[];
  const total = Number(r.headers.get("X-Total-Count") || meetings.length);
  return { meetings, total };
}

export const getMeeting = (id: string) => getJSON<Meeting>(`/api/meeting/${id}`);
export const getCommittees = () => getJSON<CommitteeListItem[]>("/api/committees");
export const getCommittee = (id: string) => getJSON<Committee>(`/api/committee/${id}`);

export interface LiveResponse {
  meeting: Meeting | null;
  meetings: Meeting[];
}
export const getLive = () => getJSON<LiveResponse>("/api/live");

export interface PersonVote {
  meeting_id: string;
  meeting_date?: string;
  body_name?: string;
  top_label?: string;
  agenda_number?: string;
  agenda_title?: string;
  result_text?: string;
  source?: string;
  vote: string;
  roll_name?: string;
}

export interface Person {
  person_id: string;
  name: string;
  party?: string;
  party_code?: string;
  memberships: { body_id: string; body_name: string; role?: string; since?: string }[];
  votes: PersonVote[];
  vote_summary: Record<string, number>;
}

export interface VorlageStation {
  meeting_id?: string;
  date?: string;
  body_name?: string;
  body_id?: string;
  meeting_title?: string;
  documents: Doc[];
  votes: Vote[];
}

export interface VorlageChain {
  vorlage: string;
  title?: string;
  own_documents: Doc[];
  stations: VorlageStation[];
}

export interface AnalyticsParty {
  code: string;
  votes: number;
  members_cast: number;
  with_line: number;
  ja: number;
  nein: number;
  enthaltung: number;
  abwesend: number;
  cohesion: number | null;
  attendance: number | null;
}

export interface Analytics {
  totals: { votes: number; with_rollcall: number; unanimous: number; contested: number };
  parties: AnalyticsParty[];
  agreement: { a: string; b: string; agree: number; n: number }[];
  dissenters: { name: string; party: string; dissents: number; votes: number }[];
  unattributed_entries: number;
}

export const getPerson = (id: string) => getJSON<Person>(`/api/person/${id}`);
export const getVorlage = (nr: string) => getJSON<VorlageChain>(`/api/vorlage/${nr}`);
export const getAnalytics = () => getJSON<Analytics>("/api/analytics");
export const versionUrl = (fileId: string, versionId: number) =>
  `/api/file/${fileId}/version/${versionId}`;

export interface RefreshResult {
  ok: boolean;
  error?: string;
  counts?: Record<string, number>;
}
export async function refreshMeeting(id: string): Promise<Meeting & { _refresh?: RefreshResult }> {
  const r = await fetch(`/api/meeting/${id}/refresh`, { method: "POST" });
  if (!r.ok) {
    let detail = `${r.status}`;
    try {
      detail = (await r.json()).detail || detail;
    } catch {
      /* ignore */
    }
    throw new Error(detail);
  }
  return r.json();
}
