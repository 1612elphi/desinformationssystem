// Package mcp is the read-only MCP server over the archive, served over
// Streamable HTTP.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

const instructions = "Durchsuchbares Archiv des Karlsruher Ratsinformationssystems " +
	"(Sitzungskalender). Gremien (committees) -> Sitzungen (meetings) -> " +
	"Tagesordnungspunkte -> Dateien (PDF documents). Use search_documents for " +
	"full-text + faceted search; document/meeting tools return full metadata. " +
	"Dates are ISO (YYYY-MM-DD). Document fields include German+English summaries, " +
	"a category, topic tags and extracted entities. Parsed vote tallies " +
	"(Abstimmungsergebnisse, incl. per-member roll-call) are available via " +
	"search_votes / get_votes and inside get_meeting."

// clamp is the pagination guard: SQLite treats a negative LIMIT as unbounded,
// so the floor of 1 matters as much as the cap on these public tools.
func clamp(n, limit int) int {
	return max(1, min(n, limit))
}

// param is one tool argument; def is its default, nil for an optional
// argument, and required arguments set req.
type param struct {
	name, typ string
	def       any
	req       bool
}

func opt(name, typ string) param        { return param{name: name, typ: typ} }
func def(name, typ string, d any) param { return param{name: name, typ: typ, def: d} }
func required(name, typ string) param   { return param{name: name, typ: typ, req: true} }

// schema builds the input schema FastMCP derives from a Python signature, so
// tools/list matches the Python server.
func schema(tool string, params ...param) map[string]any {
	props := map[string]any{}
	req := []string{}
	for _, p := range params {
		words := strings.Split(p.name, "_")
		for i, w := range words {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
		s := map[string]any{"title": strings.Join(words, " ")}
		switch {
		case p.req:
			s["type"] = p.typ
			req = append(req, p.name)
		case p.def == nil:
			s["anyOf"] = []any{map[string]any{"type": p.typ}, map[string]any{"type": "null"}}
			s["default"] = nil
		default:
			s["type"] = p.typ
			s["default"] = p.def
		}
		props[p.name] = s
	}
	out := map[string]any{"type": "object", "title": tool + "Arguments", "properties": props}
	if len(req) > 0 {
		out["required"] = req
	}
	return out
}

// text renders a tool result as FastMCP does: one text block of indented JSON
// and no structured content.
func text(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: strings.TrimSuffix(b.String(), "\n")},
	}}, nil, nil
}

func orError(v store.Row, err error, key, id string) (*mcp.CallToolResult, any, error) {
	if err == nil && v == nil {
		return text(map[string]string{"error": "not found", key: id}, nil)
	}
	return text(v, err)
}

func add[In any](s *mcp.Server, name, desc string, in map[string]any,
	h func(context.Context, In) (*mcp.CallToolResult, any, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc, InputSchema: in},
		func(ctx context.Context, _ *mcp.CallToolRequest, args In) (*mcp.CallToolResult, any, error) {
			return h(ctx, args)
		})
}

// sdkVersion reports the SDK version as the server version, as FastMCP
// reports the mcp package version.
func sdkVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/modelcontextprotocol/go-sdk" {
				return d.Version
			}
		}
	}
	return ""
}

// fold is the casefold of store.fold, for the list_people name filter.
func fold(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ß", "ss")
}

var vorlageRE = regexp.MustCompile(`^20\d{2}/\d{3,5}$`)

func NewServer(st *store.Store) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "desinformationssystem", Version: sdkVersion()},
		&mcp.ServerOptions{Instructions: instructions})

	type searchArgs struct {
		Query     string `json:"query"`
		Committee string `json:"committee"`
		DocType   string `json:"doc_type"`
		DateFrom  string `json:"date_from"`
		DateTo    string `json:"date_to"`
		Public    *bool  `json:"public"`
		Topic     string `json:"topic"`
		Submitter string `json:"submitter"`
		District  string `json:"district"`
		Street    string `json:"street"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}
	add(s, "search_documents", `Full-text + faceted search over all indexed documents.

Args:
    query: free-text search (German); matches label, full text, summaries, topics.
    committee: committee name or id (Gremium) to filter by.
    doc_type: e.g. Beschlussvorlage, Protokoll, Vorlage, Antrag, Niederschrift.
    date_from / date_to: ISO dates bounding the meeting date.
    public: True = only public items, False = only non-public, None = all.
    topic: filter to documents carrying this exact topic tag.
    submitter: filter by submitter code — SVK (Stadtverwaltung) or a fraction
        (CDU, B90, SPD, LIN, FDP, AFD, FWV, KAL, VOL, PAR, HEI).
    district: filter to documents about a Karlsruhe Stadtteil (e.g. Durlach,
        Weststadt, Innenstadt-Ost). See list_committees-style facets for values.
    street: filter to documents mentioning this Karlsruhe street/place by name.
    limit / offset: pagination (limit <= 200).
Returns: {"total": int, "results": [document, ...]}. Each document carries a
"submitters" list of such codes and, when known, a "district" Stadtteil.
`, schema("search_documents",
		opt("query", "string"), opt("committee", "string"), opt("doc_type", "string"),
		opt("date_from", "string"), opt("date_to", "string"), opt("public", "boolean"),
		opt("topic", "string"), opt("submitter", "string"), opt("district", "string"),
		opt("street", "string"), def("limit", "integer", 25), def("offset", "integer", 0)),
		func(ctx context.Context, a searchArgs) (*mcp.CallToolResult, any, error) {
			return text(st.SearchDocuments(store.SearchParams{
				Query: a.Query, Committee: a.Committee, DocType: a.DocType,
				DateFrom: a.DateFrom, DateTo: a.DateTo, Public: a.Public, Topic: a.Topic,
				Submitter: a.Submitter, District: a.District, Street: a.Street,
				Limit: clamp(a.Limit, 200), Offset: max(0, a.Offset),
			}))
		})

	type fileArgs struct {
		FileID string `json:"file_id"`
	}
	add(s, "get_document", `Return one document with full metadata, summaries, topics, entities and
(truncated) extracted text. file_id is the numeric RIS file id.`,
		schema("get_document", required("file_id", "string")),
		func(ctx context.Context, a fileArgs) (*mcp.CallToolResult, any, error) {
			d, err := st.GetDocument(a.FileID)
			if ft, ok := d["fulltext"].(string); ok {
				if r := []rune(ft); len(r) > 8000 {
					d["fulltext"] = string(r[:8000])
				}
			}
			return orError(d, err, "file_id", a.FileID)
		})

	add(s, "list_committees", "List all committees (Gremien) with member, meeting and document counts.",
		schema("list_committees"),
		func(ctx context.Context, _ struct{}) (*mcp.CallToolResult, any, error) {
			c, err := st.ListCommittees()
			return text(store.Row{"committees": c}, err)
		})

	type meetingsArgs struct {
		Committee string `json:"committee"`
		DateFrom  string `json:"date_from"`
		DateTo    string `json:"date_to"`
		Upcoming  bool   `json:"upcoming"`
		Limit     int    `json:"limit"`
	}
	add(s, "list_meetings", `List meetings (Sitzungen), optionally filtered by committee and ISO date range.
Set upcoming=True for only future sessions, sorted soonest-first.`,
		schema("list_meetings", opt("committee", "string"), opt("date_from", "string"),
			opt("date_to", "string"), def("upcoming", "boolean", false), def("limit", "integer", 50)),
		func(ctx context.Context, a meetingsArgs) (*mcp.CallToolResult, any, error) {
			f := store.MeetingFilter{Committee: a.Committee, DateFrom: a.DateFrom, DateTo: a.DateTo}
			if a.Upcoming && f.DateFrom == "" {
				f.DateFrom = time.Now().Format(time.DateOnly)
			}
			m, err := st.ListMeetings(f, a.Upcoming, clamp(a.Limit, 500), 0)
			return text(store.Row{"meetings": m}, err)
		})

	type committeeArgs struct {
		CommitteeID string `json:"committee_id"`
	}
	add(s, "get_committee", `Return a committee (Gremium) with its member roster (name, party, role),
party composition, meeting list and document count. committee_id is the RIS slug,
e.g. 'organisation-gr-32130'.`,
		schema("get_committee", required("committee_id", "string")),
		func(ctx context.Context, a committeeArgs) (*mcp.CallToolResult, any, error) {
			c, err := st.GetCommittee(a.CommitteeID)
			return orError(c, err, "committee_id", a.CommitteeID)
		})

	type meetingArgs struct {
		MeetingID string `json:"meeting_id"`
	}
	add(s, "get_meeting", `Return one meeting with its agenda items, attached documents and parsed
vote tallies ('votes': per-TOP Ja/Nein/Enthaltung + member roll-call).
meeting_id is the RIS slug, e.g. 'termin-10526'.`,
		schema("get_meeting", required("meeting_id", "string")),
		func(ctx context.Context, a meetingArgs) (*mcp.CallToolResult, any, error) {
			m, err := st.GetMeeting(a.MeetingID)
			return orError(m, err, "meeting_id", a.MeetingID)
		})

	type votesArgs struct {
		Committee      string `json:"committee"`
		DateFrom       string `json:"date_from"`
		DateTo         string `json:"date_to"`
		Member         string `json:"member"`
		Query          string `json:"query"`
		IncludeMembers bool   `json:"include_members"`
		Limit          int    `json:"limit"`
		Offset         int    `json:"offset"`
	}
	add(s, "search_votes", `Search parsed vote tallies (Abstimmungsergebnisse) across all meetings,
newest first. Tallies come from the live ticker and the official result PDFs
('source': 'live' | 'vod' | 'pdf'; pdf is authoritative, vod is read off the
council's YouTube recording).

Args:
    committee: committee name or id (Gremium) to filter by.
    date_from / date_to: ISO dates bounding the meeting date.
    member: council-member name (substring, case-insensitive); each hit then
        carries 'member_votes' with how the matching member(s) voted
        (ja / nein / enthaltung / abwesend).
    query: substring match on TOP label, result text or agenda-item title.
    include_members: True = include the full per-member roll-call per vote
        (large; default off — use get_votes/get_meeting for one meeting).
    limit / offset: pagination (limit <= 200).
Returns: {"total": int, "results": [vote, ...]} — each vote has meeting_id,
body_name, meeting_date, top_label, agenda_number/title, ja/nein/enthaltung
counts and result_text. 'members_ok': 1 if the per-member roll-call tally equals
those counts, 0 if it drifts (the vision-parsed grid is approximate — trust the
counts over the roll-call), null if no roll-call was parsed.`,
		schema("search_votes", opt("committee", "string"), opt("date_from", "string"),
			opt("date_to", "string"), opt("member", "string"), opt("query", "string"),
			def("include_members", "boolean", false), def("limit", "integer", 50),
			def("offset", "integer", 0)),
		func(ctx context.Context, a votesArgs) (*mcp.CallToolResult, any, error) {
			return text(st.SearchVotes(store.VoteSearch{
				Committee: a.Committee, DateFrom: a.DateFrom, DateTo: a.DateTo,
				Member: a.Member, Query: a.Query, IncludeMembers: a.IncludeMembers,
				Limit: clamp(a.Limit, 200), Offset: max(0, a.Offset),
			}))
		})

	add(s, "get_votes", `All parsed vote tallies for one meeting, in agenda (TOP) order, including
the full per-member roll-call. meeting_id is the RIS slug, e.g. 'termin-10526'.`,
		schema("get_votes", required("meeting_id", "string")),
		func(ctx context.Context, a meetingArgs) (*mcp.CallToolResult, any, error) {
			v, err := st.MeetingVotes(a.MeetingID)
			return text(store.Row{"meeting_id": a.MeetingID, "votes": v}, err)
		})

	type speechesArgs struct {
		MeetingID    string `json:"meeting_id"`
		AgendaAnchor string `json:"agenda_anchor"`
		Speaker      string `json:"speaker"`
		IncludeRaw   bool   `json:"include_raw"`
	}
	add(s, "get_speeches", `Debate transcript of one meeting, transcribed from the council's YouTube recording
and cleaned for reading (punctuation, fillers removed, names corrected; wording kept).
Pure chairing by the Oberbürgermeister is left out; his substantive answers are kept.

Args:
    meeting_id: RIS slug, e.g. 'termin-10574'.
    agenda_anchor: only this agenda item, e.g. 'top6' or 'top14.1'.
    speaker: substring of the speaker's name as shown on the council display
        (surname, e.g. 'Noé', 'Schütz, K.').
    include_raw: also return 'text_raw', the unedited whisper transcript.
Returns: {"speeches": [...]} in speaking order, each with speaker, party, agenda_anchor,
start_s/end_s (seconds into the video) and 'url' pointing at that moment on YouTube.
Empty if the meeting was not streamed or not yet transcribed.`,
		schema("get_speeches", required("meeting_id", "string"), opt("agenda_anchor", "string"),
			opt("speaker", "string"), def("include_raw", "boolean", false)),
		func(ctx context.Context, a speechesArgs) (*mcp.CallToolResult, any, error) {
			rows, err := st.MeetingSpeeches(a.MeetingID, a.AgendaAnchor, a.Speaker, a.IncludeRaw)
			for _, r := range rows {
				if v, _ := r["video_id"].(string); v != "" {
					start, _ := r["start_s"].(float64)
					r["url"] = fmt.Sprintf("https://www.youtube.com/watch?v=%s&t=%ds", v, int64(start))
				}
			}
			return text(store.Row{"meeting_id": a.MeetingID, "speeches": rows}, err)
		})

	type speechSearchArgs struct {
		Query    string `json:"query"`
		Speaker  string `json:"speaker"`
		Party    string `json:"party"`
		DateFrom string `json:"date_from"`
		DateTo   string `json:"date_to"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
	add(s, "search_speeches", `Search what was said in council debates (transcribed Gemeinderat recordings).

Args:
    query: case-insensitive substring of the speech text (German), e.g. 'Radweg'.
    speaker: substring of the speaker's name.
    party: party as on the roster ('GRÜNE', 'CDU', 'SPD', ...) or its 3-letter code
        (B90, CDU, SPD, AFD, FDP, FWV, VOL, LIN, KAL, PAR, HEI).
    date_from / date_to: ISO dates bounding the meeting date.
    limit / offset: pagination (limit <= 100).
Returns: {"total": int, "results": [...]} with meeting, TOP, speaker, party, a text
snippet around the match and the word count. Use get_speeches for the full text.`,
		schema("search_speeches", opt("query", "string"), opt("speaker", "string"),
			opt("party", "string"), opt("date_from", "string"), opt("date_to", "string"),
			def("limit", "integer", 20), def("offset", "integer", 0)),
		func(ctx context.Context, a speechSearchArgs) (*mcp.CallToolResult, any, error) {
			return text(st.SearchSpeeches(store.SpeechSearch{
				Query: a.Query, Speaker: a.Speaker, Party: a.Party,
				DateFrom: a.DateFrom, DateTo: a.DateTo,
				Limit: clamp(a.Limit, 100), Offset: max(0, a.Offset),
			}))
		})

	type speechAnalyticsArgs struct {
		MeetingID string `json:"meeting_id"`
	}
	add(s, "speech_analytics", `Aggregate analysis of transcribed council debates, over all transcribed meetings or one.

Returns: totals; per party (code, OB for the chair) seat share vs speaking-time share,
words, speeches, mean sentiment (-1..1), tone mix (kooperativ/sachlich/kritisch/polemisch),
German Flesch readability and mean sentence length; top speakers by time; 'said_voted':
each speaker's stance on a TOP (dafuer/dagegen/gemischt/keine) against their roll-call
vote, with a match flag; 'matrix': which party mentions which, and the mean attitude
(-1 rejecting .. +1 agreeing); distinctive words per party (log-odds, Monroe et al. 2008)
and overall frequencies; debate length per TOP next to its vote.
Stance, tone, sentiment and attitude are estimates by the TypeSafe Jev model, not facts.`,
		schema("speech_analytics", opt("meeting_id", "string")),
		func(ctx context.Context, a speechAnalyticsArgs) (*mcp.CallToolResult, any, error) {
			return text(st.SpeechAnalytics(a.MeetingID))
		})

	type recentArgs struct {
		Since string `json:"since"`
		Limit int    `json:"limit"`
	}
	add(s, "recent_documents", "Most recently downloaded documents. 'since' is an ISO datetime lower bound.",
		schema("recent_documents", opt("since", "string"), def("limit", "integer", 25)),
		func(ctx context.Context, a recentArgs) (*mcp.CallToolResult, any, error) {
			d, err := st.RecentDocuments(a.Since, clamp(a.Limit, 200), "", "", "", "")
			return text(store.Row{"documents": d}, err)
		})

	add(s, "stats", "Index statistics: counts of bodies/meetings/documents/votes and last scrape time.",
		schema("stats"),
		func(ctx context.Context, _ struct{}) (*mcp.CallToolResult, any, error) {
			return text(st.Stats())
		})

	type peopleArgs struct {
		Name string `json:"name"`
	}
	add(s, "list_people", `Council members across all committees. Optional case-insensitive name filter.
Returns person_id, name, party, party_code and committee memberships.`,
		schema("list_people", opt("name", "string")),
		func(ctx context.Context, a peopleArgs) (*mcp.CallToolResult, any, error) {
			people, err := st.ListPeople()
			if err != nil {
				return nil, nil, err
			}
			if a.Name != "" {
				needle := fold(a.Name)
				kept := []store.Row{}
				for _, p := range people {
					if n, _ := p["name"].(string); strings.Contains(fold(n), needle) {
						kept = append(kept, p)
					}
				}
				people = kept
			}
			return text(store.Row{"people": people}, nil)
		})

	type personArgs struct {
		PersonID string `json:"person_id"`
	}
	add(s, "get_person", `One council member: committee memberships plus their full roll-call voting
record (matched from parsed vote tallies) and a vote summary. person_id is the
numeric RIS people id (from list_people or committee rosters).`,
		schema("get_person", required("person_id", "string")),
		func(ctx context.Context, a personArgs) (*mcp.CallToolResult, any, error) {
			p, err := st.GetPerson(a.PersonID)
			return orError(p, err, "person_id", a.PersonID)
		})

	type vorlageArgs struct {
		Nummer string `json:"nummer"`
	}
	add(s, "get_vorlage", `Lifecycle of a Vorlage (council proposal) by its Vorlagennummer, e.g.
'2026/0324': every meeting where a document owns or mentions the number —
committee pre-discussion through council decision — with the documents and
any parsed vote result per station.`,
		schema("get_vorlage", required("nummer", "string")),
		func(ctx context.Context, a vorlageArgs) (*mcp.CallToolResult, any, error) {
			if !vorlageRE.MatchString(a.Nummer) {
				return text(map[string]string{
					"error": "invalid Vorlagennummer (expected e.g. 2026/0324)", "nummer": a.Nummer}, nil)
			}
			c, err := st.VorlageChain(a.Nummer)
			return orError(c, err, "nummer", a.Nummer)
		})

	add(s, "vote_stats", `Aggregate voting analytics across all parsed roll-call tallies: per-party
cohesion/attendance, pairwise party agreement, top dissenters (members voting
against their party line). Party attribution is surname-matched; unattributable
roll-call entries are excluded, not guessed.`,
		schema("vote_stats"),
		func(ctx context.Context, _ struct{}) (*mcp.CallToolResult, any, error) {
			return text(st.VoteAnalytics())
		})
	return s
}

// Handler serves the MCP server over Streamable HTTP at any path it is
// mounted on.
func Handler(st *store.Store) http.Handler {
	s := NewServer(st)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
}
