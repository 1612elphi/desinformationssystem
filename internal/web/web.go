// Package web is the JSON API plus the static SPA, on one origin.
package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// RefreshFunc re-scrapes one meeting and returns the ingester's result summary.
type RefreshFunc func(meetingID string) (any, error)

type Server struct {
	Store  *store.Store
	PDFDir string
	Dist   string
	// MapDir holds karlsruhe.pmtiles.
	MapDir string
	// Refresh is nil until an ingester is linked in; the endpoint is 501 then.
	Refresh RefreshFunc

	refreshing  sync.Map
	refreshSema chan struct{}

	cacheMu sync.Mutex
	cache   map[string]cached
}

type cached struct {
	at   time.Time
	data store.Row
}

// httpError carries a status code and FastAPI-style detail.
type httpError struct {
	code   int
	detail string
}

func (e httpError) Error() string { return e.detail }

func fail(code int, detail string) error { return httpError{code, detail} }

var errNotFound = fail(http.StatusNotFound, "not found")

type handler func(w http.ResponseWriter, r *http.Request) (any, error)

func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := h(w, r)
		if err != nil {
			var he httpError
			if !errors.As(err, &he) {
				log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
				he = httpError{http.StatusInternalServerError, "internal error"}
			}
			writeJSON(w, he.code, map[string]string{"detail": he.detail})
			return
		}
		if v != nil {
			writeJSON(w, http.StatusOK, v)
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func found(v store.Row, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errNotFound
	}
	return v, nil
}

// intParam reads an integer query parameter bounded to [lo, hi]; out of range
// is a 422 like FastAPI's Query(ge, le) validation.
func intParam(r *http.Request, name string, def, lo, hi int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, fail(http.StatusUnprocessableEntity, fmt.Sprintf("%s must be an integer in [%d, %d]", name, lo, hi))
	}
	return n, nil
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

var (
	meetingIDRE = regexp.MustCompile(`^termin-\d+$`)
	personIDRE  = regexp.MustCompile(`^\d{1,10}$`)
	yearRE      = regexp.MustCompile(`^20\d{2}$`)
	seqRE       = regexp.MustCompile(`^\d{3,5}$`)
)

func (s *Server) Handler() http.Handler {
	if s.refreshSema == nil {
		// cap concurrent refreshes; the endpoint is public and unauthenticated
		s.refreshSema = make(chan struct{}, 2)
	}
	mux := http.NewServeMux()
	h := func(pattern string, fn handler) { mux.Handle(pattern, s.wrap(fn)) }

	h("GET /health", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	h("GET /api/stats", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.Store.Stats() })
	h("GET /api/facets", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.Store.Facets() })
	h("GET /api/search", s.search)
	h("GET /api/document/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return found(s.Store.GetDocument(r.PathValue("id")))
	})
	h("GET /api/committees", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.Store.ListCommittees() })
	h("GET /api/meetings", s.meetings)
	h("GET /api/votes", s.votes)
	h("GET /api/speeches/search", s.speechSearch)
	h("GET /api/meetings.ics", s.ics)
	h("GET /api/meeting/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return found(s.Store.GetMeeting(r.PathValue("id")))
	})
	h("GET /api/meeting/{id}/speeches", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.Store.MeetingSpeeches(r.PathValue("id"), "", "", truthy(r.URL.Query().Get("raw")))
	})
	h("POST /api/meeting/{id}/refresh", s.refresh)
	h("GET /api/committee/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return found(s.Store.GetCommittee(r.PathValue("id")))
	})
	h("GET /api/live", func(w http.ResponseWriter, r *http.Request) (any, error) {
		live, err := s.Store.LiveMeetings(time.Now())
		if err != nil {
			return nil, err
		}
		var primary any
		if len(live) > 0 {
			primary = live[0]
		}
		return map[string]any{"meeting": primary, "meetings": live}, nil
	})
	h("GET /api/recent", func(w http.ResponseWriter, r *http.Request) (any, error) {
		limit, err := intParam(r, "limit", 50, 1, 200)
		if err != nil {
			return nil, err
		}
		return s.Store.RecentDocuments(r.URL.Query().Get("since"), limit, "", "", "", "")
	})
	h("GET /api/people", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.Store.ListPeople() })
	h("GET /api/person/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id := r.PathValue("id")
		if !personIDRE.MatchString(id) {
			return nil, fail(http.StatusBadRequest, "invalid person id")
		}
		return found(s.Store.GetPerson(id))
	})
	h("GET /api/vorlage/{year}/{seq}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		y, q := r.PathValue("year"), r.PathValue("seq")
		if !yearRE.MatchString(y) || !seqRE.MatchString(q) {
			return nil, fail(http.StatusBadRequest, "invalid Vorlagennummer")
		}
		return found(s.Store.VorlageChain(y + "/" + q))
	})
	h("GET /api/places", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		return s.Store.Places(store.PlaceFilter{
			DateFrom: q.Get("from"), DateTo: q.Get("to"), Committee: q.Get("committee"), Kind: q.Get("kind"),
		})
	})
	h("GET /api/places/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			return nil, fail(http.StatusBadRequest, "invalid place id")
		}
		return found(s.Store.GetPlace(id))
	})
	h("GET /api/analytics", s.analyticsHandler)
	h("GET /api/speeches/analytics", s.speechAnalytics)
	h("GET /api/feed.xml", s.feed)
	h("GET /api/file/{id}", s.file)
	h("GET /api/file/{id}/version/{vid}", s.fileVersion)
	mux.Handle("/api/", s.wrap(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return nil, errNotFound
	}))
	mux.HandleFunc("GET /map/karlsruhe.pmtiles", s.mapTiles)
	mux.Handle("/", s.spa())
	return mux
}

// mapTiles serves the PMTiles archive; the map client reads it with Range
// requests, which ServeContent answers.
func (s *Server) mapTiles(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(filepath.Join(s.MapDir, "karlsruhe.pmtiles"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.pmtiles")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	limit, err := intParam(r, "limit", 50, 1, 200)
	if err != nil {
		return nil, err
	}
	offset, err := intParam(r, "offset", 0, 0, 1<<31-1)
	if err != nil {
		return nil, err
	}
	p := store.SearchParams{
		Query: q.Get("q"), Committee: q.Get("committee"), DocType: q.Get("type"),
		DateFrom: q.Get("from"), DateTo: q.Get("to"), Topic: q.Get("topic"),
		Submitter: q.Get("submitter"), District: q.Get("district"), Street: q.Get("street"),
		Category: q.Get("category"), Facets: truthy(q.Get("facets")),
		Limit: limit, Offset: offset,
	}
	switch pub := q.Get("public"); pub {
	case "", "all":
	default:
		b := pub == "1" || pub == "true" || pub == "public" || pub == "öffentlich"
		p.Public = &b
	}
	res, err := s.Store.SearchDocuments(p)
	if err != nil && strings.Contains(err.Error(), "fts5") {
		return nil, fail(http.StatusBadRequest, "invalid search query")
	}
	return res, err
}

func (s *Server) meetings(w http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	limit, err := intParam(r, "limit", 100, 1, 500)
	if err != nil {
		return nil, err
	}
	offset, err := intParam(r, "offset", 0, 0, 1<<31-1)
	if err != nil {
		return nil, err
	}
	f := store.MeetingFilter{Committee: q.Get("committee"), DateFrom: q.Get("from"), DateTo: q.Get("to"), Query: q.Get("q")}
	order := q.Get("order")
	if truthy(q.Get("upcoming")) {
		if f.DateFrom == "" {
			f.DateFrom = time.Now().Format(time.DateOnly)
		}
		if order == "" {
			order = "asc"
		}
	}
	n, err := s.Store.CountMeetings(f)
	if err != nil {
		return nil, err
	}
	w.Header().Set("X-Total-Count", strconv.FormatInt(n, 10))
	return s.Store.ListMeetings(f, strings.EqualFold(order, "asc"), limit, offset)
}

// page reads limit and offset; an over-cap limit is clamped as the MCP tools
// clamp it, a non-integer or a limit below 1 is a 422.
func page(r *http.Request, def, most int) (limit, offset int, err error) {
	if limit, err = intParam(r, "limit", def, 1, 1<<31-1); err != nil {
		return 0, 0, err
	}
	offset, err = intParam(r, "offset", 0, 0, 1<<31-1)
	return min(limit, most), offset, err
}

// votes is the MCP search_votes tool over HTTP.
func (s *Server) votes(w http.ResponseWriter, r *http.Request) (any, error) {
	limit, offset, err := page(r, 50, 200)
	if err != nil {
		return nil, err
	}
	q := r.URL.Query()
	return s.Store.SearchVotes(store.VoteSearch{
		Query: q.Get("q"), Committee: q.Get("committee"), DateFrom: q.Get("from"),
		DateTo: q.Get("to"), Member: q.Get("member"), IncludeMembers: truthy(q.Get("include_members")),
		Limit: limit, Offset: offset,
	})
}

// speechSearch is the MCP search_speeches tool over HTTP.
func (s *Server) speechSearch(w http.ResponseWriter, r *http.Request) (any, error) {
	limit, offset, err := page(r, 20, 100)
	if err != nil {
		return nil, err
	}
	q := r.URL.Query()
	return s.Store.SearchSpeeches(store.SpeechSearch{
		Query: q.Get("q"), Speaker: q.Get("speaker"), Party: q.Get("party"),
		Meeting: q.Get("meeting"), DateFrom: q.Get("from"), DateTo: q.Get("to"),
		Limit: limit, Offset: offset,
	})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) (any, error) {
	id := r.PathValue("id")
	if !meetingIDRE.MatchString(id) {
		return nil, fail(http.StatusBadRequest, "invalid meeting id")
	}
	if s.Refresh == nil {
		return nil, fail(http.StatusNotImplemented, "refresh not available")
	}
	if _, busy := s.refreshing.LoadOrStore(id, true); busy {
		return nil, fail(http.StatusConflict, "refresh already running")
	}
	defer s.refreshing.Delete(id)
	s.refreshSema <- struct{}{}
	result, err := s.Refresh(id)
	<-s.refreshSema
	if err != nil {
		var he httpError
		if errors.As(err, &he) {
			return nil, he
		}
		return nil, fail(http.StatusBadGateway, "refresh failed: "+err.Error())
	}
	m, err := found(s.Store.GetMeeting(id))
	if err != nil {
		return nil, err
	}
	m.(store.Row)["_refresh"] = result
	return m, nil
}

// cachedRow returns load's result, kept 10 min per key.
// ponytail: one lock for all keys, a slow load blocks the others; per-key locks if that shows
func (s *Server) cachedRow(key string, load func() (store.Row, error)) (store.Row, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if hit, ok := s.cache[key]; ok && time.Since(hit.at) <= 10*time.Minute {
		return hit.data, nil
	}
	a, err := load()
	if err != nil {
		return nil, err
	}
	if s.cache == nil {
		s.cache = map[string]cached{}
	}
	s.cache[key] = cached{time.Now(), a}
	return a, nil
}

// analyticsHandler is cached since the aggregation walks every roll-call.
func (s *Server) analyticsHandler(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.cachedRow("votes", s.Store.VoteAnalytics)
}

// speechAnalytics is the "Reden" tab, cached per meeting filter.
func (s *Server) speechAnalytics(w http.ResponseWriter, r *http.Request) (any, error) {
	key := r.URL.Query().Get("meeting")
	return s.cachedRow("speeches:"+key, func() (store.Row, error) { return s.Store.SpeechAnalytics(key) })
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) (any, error) {
	id := r.PathValue("id")
	doc, err := found(s.Store.GetDocument(id))
	if err != nil {
		return nil, err
	}
	d := doc.(store.Row)
	if p, _ := d["local_path"].(string); p != "" {
		if _, err := os.Stat(p); err == nil {
			disp := "inline"
			if truthy(r.URL.Query().Get("download")) {
				disp = "attachment"
			}
			name, _ := d["filename"].(string)
			if name == "" {
				name = id + ".pdf"
			}
			servePDF(w, r, p, disp, name)
			return nil, nil
		}
	}
	if u, _ := d["url"].(string); strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		w.Header().Set("Location", u)
		writeJSON(w, http.StatusFound, map[string]string{"redirect": u})
		return nil, nil
	}
	return nil, fail(http.StatusNotFound, "file not cached")
}

func (s *Server) fileVersion(w http.ResponseWriter, r *http.Request) (any, error) {
	id := r.PathValue("id")
	vid, err := strconv.ParseInt(r.PathValue("vid"), 10, 64)
	if err != nil {
		return nil, fail(http.StatusUnprocessableEntity, "invalid version id")
	}
	v, err := s.Store.GetFileVersion(vid)
	if err != nil {
		return nil, err
	}
	if v == nil || v["file_id"] != id {
		return nil, errNotFound
	}
	p, _ := v["local_path"].(string)
	// archived files are always under PDF_DIR/versions; refuse anything else
	vdir, _ := filepath.EvalSymlinks(filepath.Join(s.PDFDir, "versions"))
	real, err := filepath.EvalSymlinks(p)
	if p == "" || err != nil || vdir == "" || !strings.HasPrefix(real, vdir+string(filepath.Separator)) {
		return nil, fail(http.StatusNotFound, "version file missing")
	}
	stamp, _ := v["superseded_at"].(string)
	name := id + ".old.pdf"
	if len(stamp) >= 10 {
		name = id + "." + stamp[:10] + ".pdf"
	}
	servePDF(w, r, real, "inline", name)
	return nil, nil
}

func servePDF(w http.ResponseWriter, r *http.Request, path, disposition, name string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, name))
	http.ServeFile(w, r, path)
}

// spa serves the built frontend; extensionless paths that are not files get
// index.html so client-side routes resolve, missing assets stay 404.
func (s *Server) spa() http.Handler {
	fs := http.FileServer(http.Dir(s.Dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(s.Dist, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if st, err := os.Stat(p); (err != nil && filepath.Ext(p) == "") || (err == nil && st.IsDir()) {
			// the page names the current hashed bundle; a cached copy would load an old one
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(s.Dist, "index.html"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}

// rfc2822 formats a SQLite datetime('now') value (UTC) for RSS pubDate.
func rfc2822(sqliteUTC string) string {
	t, err := time.Parse(time.DateTime, sqliteUTC)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", sqliteUTC)
		if err != nil {
			return ""
		}
	}
	return t.UTC().Format("Mon, 02 Jan 2006 15:04:05 +0000")
}
