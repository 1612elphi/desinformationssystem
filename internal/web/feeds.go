package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var icsEscaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)

func str(v any) string {
	s, _ := v.(string)
	return s
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ics is a subscribable calendar of recent and upcoming sessions.
func (s *Server) ics(w http.ResponseWriter, r *http.Request) (any, error) {
	committee := r.URL.Query().Get("committee")
	start := time.Now().AddDate(0, 0, -30).Format(time.DateOnly)
	meetings, err := s.Store.ListMeetings(store.MeetingFilter{Committee: committee, DateFrom: start}, true, 500, 0)
	if err != nil {
		return nil, err
	}
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//desinformationssystem//karlsruhe//DE",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:" + icsEscaper.Replace("Karlsruhe "+firstOf(committee, "Sitzungskalender")),
		"X-WR-TIMEZONE:Europe/Berlin",
		"BEGIN:VTIMEZONE",
		"TZID:Europe/Berlin",
		"BEGIN:DAYLIGHT",
		"TZOFFSETFROM:+0100", "TZOFFSETTO:+0200", "TZNAME:CEST",
		"DTSTART:19700329T020000", "RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU",
		"END:DAYLIGHT",
		"BEGIN:STANDARD",
		"TZOFFSETFROM:+0200", "TZOFFSETTO:+0100", "TZNAME:CET",
		"DTSTART:19701025T030000", "RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU",
		"END:STANDARD",
		"END:VTIMEZONE",
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, m := range meetings {
		day, err := time.Parse(time.DateOnly, str(m["date"]))
		if err != nil {
			continue
		}
		lines = append(lines, "BEGIN:VEVENT", "UID:"+str(m["id"])+"@dis.delphi.tools", "DTSTAMP:"+stamp)
		if st, en, ok := store.ParseTimeWindow(str(m["time"]), day); ok {
			lines = append(lines,
				"DTSTART;TZID=Europe/Berlin:"+st.Format("20060102T150405"),
				"DTEND;TZID=Europe/Berlin:"+en.Format("20060102T150405"))
		} else {
			lines = append(lines, "DTSTART;VALUE=DATE:"+day.Format("20060102"))
		}
		lines = append(lines, "SUMMARY:"+icsEscaper.Replace(firstOf(str(m["body_name"]), str(m["title"]), "Sitzung")))
		if loc := str(m["location"]); loc != "" {
			lines = append(lines, "LOCATION:"+icsEscaper.Replace(loc))
		}
		if u := str(m["url"]); u != "" {
			lines = append(lines, "URL:"+u)
		}
		lines = append(lines, "DESCRIPTION:"+icsEscaper.Replace(str(m["title"])+" · "+str(m["url"])), "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="karlsruhe.ics"`)
	// RFC 5545 section 3.1: lines end in CRLF
	_, _ = w.Write([]byte(strings.Join(lines, "\r\n") + "\r\n"))
	return nil, nil
}

func publicBase(r *http.Request) string {
	host := firstOf(r.Host, "dis.delphi.tools")
	if strings.Contains(host, "delphi.tools") {
		return "https://" + host
	}
	return "http://" + host
}

var esc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace

// feed is an RSS 2.0 feed of newly archived documents.
func (s *Server) feed(w http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	limit, err := intParam(r, "limit", 50, 1, 100)
	if err != nil {
		return nil, err
	}
	committee, docType, submitter, topic := q.Get("committee"), q.Get("type"), q.Get("submitter"), q.Get("topic")
	docs, err := s.Store.RecentDocuments("", limit, committee, docType, submitter, topic)
	if err != nil {
		return nil, err
	}
	base := publicBase(r)
	title := "Desinformationssystem Karlsruhe"
	var bits []string
	for _, f := range []string{committee, docType, submitter, topic} {
		if f != "" {
			bits = append(bits, f)
		}
	}
	if len(bits) > 0 {
		title += " — " + strings.Join(bits, " · ")
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel>`)
	b.WriteString("<title>" + esc(title) + "</title><link>" + esc(base) + "</link>")
	b.WriteString("<description>Neu archivierte Dokumente aus dem Karlsruher Ratsinformationssystem</description>")
	b.WriteString("<language>de-de</language>")
	for _, d := range docs {
		id := str(d["id"])
		var parts []string
		for _, k := range []string{"doc_type", "body_name", "meeting_date"} {
			if v := str(d[k]); v != "" {
				parts = append(parts, v)
			}
		}
		desc := strings.Join(parts, " · ")
		if sum := str(d["summary_de"]); sum != "" {
			desc += "\n\n" + sum
		}
		b.WriteString("<item><title>" + esc(firstOf(str(d["label"]), id)) + "</title>")
		b.WriteString("<link>" + esc(base+"/?doc="+id) + "</link>")
		b.WriteString(`<guid isPermaLink="false">` + esc(id) + "</guid>")
		if pub := rfc2822(str(d["downloaded_at"])); pub != "" {
			b.WriteString("<pubDate>" + pub + "</pubDate>")
		}
		b.WriteString("<description>" + esc(desc) + "</description></item>")
	}
	b.WriteString("</channel></rss>")
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
	return nil, nil
}
