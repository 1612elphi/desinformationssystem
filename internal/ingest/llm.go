package ingest

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var (
	OpenRouterBase = envStr("OPENROUTER_BASE", "https://openrouter.ai/api/v1")
	OpenRouterKey  = envStr("OPENROUTER_API_KEY", "")
	LLMModel       = envStr("LLM_MODEL", "deepseek/deepseek-v4-pro")
	EnrichMaxChars = envInt("ENRICH_MAXCHARS", 24000)
	LLMTimeout     = envSeconds("LLM_TIMEOUT", 120)
	LLMRetries     = envInt("LLM_RETRIES", 3)
	LLMMinInterval = envSeconds("LLM_MIN_INTERVAL", 1.0)
	HTTPReferer    = envStr("OPENROUTER_REFERER", "https://desinformationssystem.local")
	VisionModel    = envStr("VISION_MODEL", "google/gemini-3.1-flash-lite")
	VisionTimeout  = envSeconds("VISION_TIMEOUT", 90)
	VisionRetries  = envInt("VISION_RETRIES", 3)
	// retry backoff unit, a variable so tests do not wait
	backoffUnit = time.Second
)

const appTitle = "Desinformationssystem"

var Categories = []string{
	"Bauen & Stadtentwicklung",
	"Umwelt & Klima",
	"Verkehr & Mobilität",
	"Soziales & Gesundheit",
	"Bildung & Kultur",
	"Finanzen & Haushalt",
	"Wirtschaft & Wissenschaft",
	"Verwaltung & Organisation",
	"Sicherheit & Ordnung",
	"Sonstiges",
}

const systemPrompt = "Du bist ein Archivar, der Dokumente aus dem Karlsruher Ratsinformationssystem " +
	"erschließt. Antworte ausschließlich mit einem JSON-Objekt, keine Erklärungen."

const votePrompt = "Dies ist ein Abstimmungspanel eines deutschen Gemeinderats (\"SITZUNG LIVE\"). " +
	"Oben rechts stehen drei große Zähler: JA, NEIN, ENTHALTUNG. Darunter ist ein Raster " +
	"der Mitglieder, gruppiert nach Fraktion; die Kachelfarbe codiert die Stimme: " +
	"GELB = Ja, ROT = Nein, GRAU = Enthaltung/abwesend. " +
	"Gib NUR ein JSON-Objekt zurück, keine Erklärung, mit den Feldern: " +
	"ja (int), nein (int), enthaltung (int), " +
	"members (Liste von {\"name\": str, \"vote\": \"ja\"|\"nein\"|\"enthaltung\"|\"abwesend\"}). " +
	"Lies die drei Zähler exakt ab; members nur für klar erkennbare farbige Kacheln."

func EnrichEnabled() bool { return OpenRouterKey != "" && envStr("ENABLE_LLM", "1") == "1" }

func VoteParseEnabled() bool {
	return OpenRouterKey != "" && envStr("ENABLE_VOTE_PARSE", "1") == "1"
}

var (
	throttleMu sync.Mutex
	lastLLM    time.Time
)

func llmThrottle() {
	throttleMu.Lock()
	defer throttleMu.Unlock()
	if wait := LLMMinInterval - time.Since(lastLLM); wait > 0 {
		time.Sleep(wait)
	}
	lastLLM = time.Now()
}

// truthy is Python's bool() on a decoded JSON value.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		f, err := x.Float64()
		return err != nil || f != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

// pyStr is Python's str() on a decoded JSON scalar.
func pyStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case nil:
		return "None"
	case json.Number:
		if !strings.ContainsAny(string(x), ".eE") {
			return string(x)
		}
		f, _ := x.Float64()
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	return fmt.Sprint(v)
}

func docStr(d store.Row, key, def string) string {
	if v := d[key]; truthy(v) {
		return fmt.Sprint(v)
	}
	return def
}

func buildPrompt(d store.Row) string {
	text := runeHead(docStr(d, "fulltext", ""), EnrichMaxChars)
	return "Kontext:\n" +
		"- Gremium: " + docStr(d, "body_name", "unbekannt") + "\n" +
		"- Sitzung: " + docStr(d, "meeting_title", "") + " (" + docStr(d, "meeting_date", "") + ")\n" +
		"- Dokumenttyp: " + docStr(d, "doc_type", "") + "\n" +
		"- Bezeichnung: " + docStr(d, "label", "") + "\n\n" +
		"Dokumenttext (ggf. gekürzt):\n\"\"\"\n" + text + "\n\"\"\"\n\n" +
		"Erzeuge ein JSON-Objekt mit genau diesen Feldern:\n" +
		"  summary_de: 2-4 Sätze deutsche Zusammenfassung des Inhalts.\n" +
		"  summary_en: dieselbe Zusammenfassung auf Englisch.\n" +
		"  category: GENAU einer dieser Werte: " + strings.Join(Categories, ", ") + ".\n" +
		"  topics: Liste von 3-7 kurzen deutschen Schlagworten.\n" +
		"  entities: Objekt mit den Listen people, orgs, locations " +
		"(Personen, Organisationen/Fraktionen/Ämter, Orte/Straßen), je 0-8 Einträge.\n" +
		"Wenn der Text leer oder unbrauchbar ist, gib leere Zusammenfassungen und " +
		"category \"Sonstiges\" zurück."
}

func decodeJSON(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data after JSON value")
	}
	return v, nil
}

// parseModelJSON accepts a bare object, a ```-fenced one, or an object
// embedded in prose.
func parseModelJSON(content string) (map[string]any, error) {
	content = pyStrip(content)
	if strings.HasPrefix(content, "```") {
		content = strings.Trim(content, "`")
		i := strings.Index(content, "{")
		if i < 0 {
			i = max(0, len(content)-1)
		}
		content = content[i:]
	}
	v, err := decodeJSON(content)
	if err != nil {
		s, e := strings.Index(content, "{"), strings.LastIndex(content, "}")
		if s < 0 || e <= s {
			return nil, err
		}
		if v, err = decodeJSON(content[s : e+1]); err != nil {
			return nil, err
		}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("model returned %T, not an object", v)
	}
	return m, nil
}

var noRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// chatCompletion posts one request and returns choices[0].message.content.
func chatCompletion(payload any, headers map[string]string, timeout time.Duration) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, OpenRouterBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: timeout, CheckRedirect: noRedirect}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == nil {
		return "", errors.New("response has no message content")
	}
	return *out.Choices[0].Message.Content, nil
}

func callOpenRouter(prompt string) (map[string]any, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + OpenRouterKey,
		"HTTP-Referer":  HTTPReferer,
		"X-Title":       appTitle,
		"Content-Type":  "application/json",
	}
	payload := map[string]any{
		"model": LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.2,
	}
	var last error
	for attempt := range LLMRetries {
		llmThrottle()
		content, err := chatCompletion(payload, headers, LLMTimeout)
		if err == nil {
			var m map[string]any
			if m, err = parseModelJSON(content); err == nil {
				return m, nil
			}
		}
		last = err
		log.Printf("enrich: openrouter attempt %d/%d failed: %v", attempt+1, LLMRetries, err)
		time.Sleep(min(30*backoffUnit, backoffUnit<<attempt))
	}
	return nil, fmt.Errorf("openrouter failed after %d attempts: %v", LLMRetries, last)
}

// strList coerces a model value into at most limit non-empty strings; the
// model sometimes returns a bare string or objects inside the list.
func strList(v any, limit int) []string {
	if s, ok := v.(string); ok {
		parts := strings.Split(s, ",")
		items := make([]any, len(parts))
		for i, p := range parts {
			items[i] = pyStrip(p)
		}
		v = items
	}
	list, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := []string{}
	for _, t := range list {
		if d, ok := t.(map[string]any); ok {
			switch {
			case truthy(d["name"]):
				t = d["name"]
			case truthy(d["value"]):
				t = d["value"]
			default:
				t = ""
			}
		}
		switch t.(type) {
		case string, json.Number, bool:
			if s := pyStrip(pyStr(t)); s != "" {
				out = append(out, s)
			}
		}
	}
	return out[:min(len(out), limit)]
}

type enrichment struct {
	SummaryDE, SummaryEN, Category string
	Topics                         []string
	Entities                       store.Obj
}

func normaliseEnrichment(r map[string]any) enrichment {
	cat := "Sonstiges"
	if truthy(r["category"]) {
		if s, ok := r["category"].(string); ok {
			cat = pyStrip(s)
		}
	}
	known := false
	for _, c := range Categories {
		known = known || c == cat
	}
	if !known {
		match := "Sonstiges"
		for _, c := range Categories {
			if strings.ToLower(c) == strings.ToLower(cat) {
				match = c
				break
			}
		}
		cat = match
	}
	ents, _ := r["entities"].(map[string]any)
	entities := store.Obj{}
	for _, k := range []string{"people", "orgs", "locations"} {
		entities = append(entities, struct {
			K string
			V any
		}{k, strList(ents[k], 8)})
	}
	s := func(v any) string {
		if x, ok := v.(string); ok {
			return pyStrip(x)
		}
		return ""
	}
	return enrichment{s(r["summary_de"]), s(r["summary_en"]), cat, strList(r["topics"], 7), entities}
}

func setEnrichStatus(st *store.Store, status string, ids ...string) error {
	return st.Write(func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE files SET enrich_status=? WHERE id=?", status, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// EnrichFile asks the LLM for summaries, category, topics and entities and
// sets enrich_status to ok, skipped (disabled) or error.
func EnrichFile(st *store.Store, id string) (bool, error) {
	doc, err := st.GetDocument(id)
	if err != nil || doc == nil {
		return false, err
	}
	if !EnrichEnabled() {
		return false, setEnrichStatus(st, "skipped", id)
	}
	raw, err := callOpenRouter(buildPrompt(doc))
	if err != nil {
		log.Printf("enrich: enrich error for %s: %v", id, err)
		return false, setEnrichStatus(st, "error", id)
	}
	return true, writeEnrichment(st, id, normaliseEnrichment(raw))
}

func writeEnrichment(st *store.Store, id string, e enrichment) error {
	return st.Write(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE files SET summary_de=?, summary_en=?, category=?, topics=?,
				entities=?, enrich_status='ok', enriched_at=datetime('now')
			WHERE id=?`, e.SummaryDE, e.SummaryEN, e.Category, store.PyJSON(e.Topics),
			store.PyJSON(e.Entities), id); err != nil {
			return err
		}
		return store.ReindexFile(tx, id)
	})
}

// VoteTally is a parsed vote panel.
type VoteTally struct {
	Ja, Nein, Enthaltung *int
	Members              []store.VoteMember
}

func toInt(v any) *int {
	switch v.(type) {
	case string, json.Number:
	default:
		return nil
	}
	f, err := strconv.ParseFloat(pyStrip(pyStr(v)), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return intp(int(math.RoundToEven(f)))
}

var voteAliases = map[string]string{
	"ja": "ja", "yes": "ja", "dafür": "ja", "zustimmung": "ja",
	"nein": "nein", "no": "nein", "dagegen": "nein", "ablehnung": "nein",
	"enthaltung": "enthaltung", "enthalten": "enthaltung", "abstention": "enthaltung",
	"abwesend": "abwesend", "absent": "abwesend", "nicht anwesend": "abwesend",
}

func normVote(v any) string {
	s := ""
	if truthy(v) {
		s = pyStr(v)
	}
	if a, ok := voteAliases[pyStrip(strings.ToLower(s))]; ok {
		return a
	}
	return "abwesend"
}

func normaliseTally(raw map[string]any) *VoteTally {
	t := &VoteTally{Ja: toInt(raw["ja"]), Nein: toInt(raw["nein"]), Enthaltung: toInt(raw["enthaltung"]),
		Members: []store.VoteMember{}}
	if t.Ja == nil && t.Nein == nil && t.Enthaltung == nil {
		return nil
	}
	members, _ := raw["members"].([]any)
	for _, m := range members {
		if d, ok := m.(map[string]any); ok && truthy(d["name"]) {
			t.Members = append(t.Members, store.VoteMember{Name: pyStrip(pyStr(d["name"])), Vote: normVote(d["vote"])})
		}
	}
	return t
}

// ParseVoteImage reads a vote panel image with the vision model; nil on
// failure or when disabled.
func ParseVoteImage(image []byte, mime string) *VoteTally {
	if !VoteParseEnabled() {
		return nil
	}
	payload := map[string]any{
		"model":       VisionModel,
		"temperature": 0,
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": votePrompt},
			map[string]any{"type": "image_url", "image_url": map[string]string{
				"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image)}},
		}}},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + OpenRouterKey,
		"X-Title":       appTitle,
		"Content-Type":  "application/json",
	}
	var last error
	for attempt := range VisionRetries {
		content, err := chatCompletion(payload, headers, VisionTimeout)
		if err == nil {
			var raw map[string]any
			if raw, err = parseModelJSON(content); err == nil {
				return normaliseTally(raw)
			}
		}
		last = err
		log.Printf("vote_parse: vote parse attempt %d/%d failed: %v", attempt+1, VisionRetries, err)
		time.Sleep(min(20*backoffUnit, backoffUnit<<attempt))
	}
	log.Printf("vote_parse: vote parse gave up: %v", last)
	return nil
}
