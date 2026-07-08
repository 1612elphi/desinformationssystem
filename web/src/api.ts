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

export function getMeetings(p: {
  committee?: string;
  upcoming?: boolean;
  from?: string;
  to?: string;
  limit?: number;
}): Promise<Meeting[]> {
  const qs = new URLSearchParams();
  if (p.committee) qs.set("committee", p.committee);
  if (p.upcoming) qs.set("upcoming", "true");
  if (p.from) qs.set("from", p.from);
  if (p.to) qs.set("to", p.to);
  qs.set("limit", String(p.limit ?? 200));
  return getJSON<Meeting[]>(`/api/meetings?${qs.toString()}`);
}

export const getMeeting = (id: string) => getJSON<Meeting>(`/api/meeting/${id}`);
export const getCommittees = () => getJSON<CommitteeListItem[]>("/api/committees");
export const getCommittee = (id: string) => getJSON<Committee>(`/api/committee/${id}`);

export interface LiveResponse {
  meeting: Meeting | null;
  meetings: Meeting[];
}
export const getLive = () => getJSON<LiveResponse>("/api/live");

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
