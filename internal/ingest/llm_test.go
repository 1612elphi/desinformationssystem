package ingest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// fakeOpenRouter answers each chat completion with the next reply; a reply
// starting with "status " is sent as that HTTP status.
func fakeOpenRouter(t *testing.T, replies ...string) (*[]map[string]any, *[]http.Header) {
	t.Helper()
	var bodies []map[string]any
	var headers []http.Header
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(b, &body)
		bodies = append(bodies, body)
		headers = append(headers, r.Header.Clone())
		reply := replies[min(int(n.Add(1))-1, len(replies)-1)]
		if reply == "status 500" {
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	oldBase, oldKey, oldUnit, oldInterval := OpenRouterBase, OpenRouterKey, backoffUnit, LLMMinInterval
	OpenRouterBase, OpenRouterKey, backoffUnit, LLMMinInterval = srv.URL, "test-key", 0, 0
	t.Cleanup(func() {
		OpenRouterBase, OpenRouterKey, backoffUnit, LLMMinInterval = oldBase, oldKey, oldUnit, oldInterval
	})
	return &bodies, &headers
}

func TestEnrichFile(t *testing.T) {
	st := testStore(t)
	mustExec(t, st, `INSERT INTO meetings(id, body_name, title, date) VALUES('termin-1','Gemeinderat','Gemeinderat (öffentlich)','2026-07-28')`)
	mustExec(t, st, `INSERT INTO files(id, url, meeting_id, label, doc_type, fulltext, text_status) VALUES('42','u','termin-1','Antrag Radweg','Antrag','Der Radweg in der Kaiserstraße.','ok')`)
	reply := "```json\n" + `{"summary_de": " Ein Radweg. ", "summary_en": "A bike lane.", "category": "verkehr & mobilität",
		"topics": "Radweg, Kaiserstraße, ", "entities": {"people": [{"name": "Dr. Mentrup"}, 7, true], "orgs": "Stadt", "locations": null}}` + "\n```"
	bodies, headers := fakeOpenRouter(t, "status 500", reply)
	ok, err := EnrichFile(st, "42")
	if err != nil || !ok {
		t.Fatalf("EnrichFile = %v, %v", ok, err)
	}
	if len(*bodies) != 2 {
		t.Fatalf("want one retry, got %d requests", len(*bodies))
	}
	h := (*headers)[1]
	if h.Get("Authorization") != "Bearer test-key" || h.Get("X-Title") != "Desinformationssystem" || h.Get("HTTP-Referer") == "" {
		t.Errorf("headers: %v", h)
	}
	b := (*bodies)[1]
	msgs := b["messages"].([]any)
	user := msgs[1].(map[string]any)["content"].(string)
	if b["model"] != LLMModel || b["temperature"] != 0.2 || msgs[0].(map[string]any)["content"] != systemPrompt ||
		!strings.Contains(user, "- Gremium: Gemeinderat\n- Sitzung: Gemeinderat (öffentlich) (2026-07-28)\n- Dokumenttyp: Antrag\n") ||
		!strings.Contains(user, "\"\"\"\nDer Radweg in der Kaiserstraße.\n\"\"\"") {
		t.Errorf("request body: %v", b)
	}
	var sumDE, cat, topics, ents, status string
	st.DB.QueryRow("SELECT summary_de, category, topics, entities, enrich_status FROM files WHERE id='42'").Scan(&sumDE, &cat, &topics, &ents, &status)
	want := []string{"Ein Radweg.", "Verkehr & Mobilität", `["Radweg", "Kaiserstraße"]`,
		`{"people": ["Dr. Mentrup", "7", "True"], "orgs": ["Stadt"], "locations": []}`, "ok"}
	if got := []string{sumDE, cat, topics, ents, status}; !reflect.DeepEqual(got, want) {
		t.Errorf("stored\n got %q\nwant %q", got, want)
	}
}

func TestEnrichFileWithoutKeySkips(t *testing.T) {
	st := testStore(t)
	mustExec(t, st, `INSERT INTO files(id, url) VALUES('1','u')`)
	old := OpenRouterKey
	OpenRouterKey = ""
	defer func() { OpenRouterKey = old }()
	if ok, err := EnrichFile(st, "1"); ok || err != nil {
		t.Fatalf("got %v %v", ok, err)
	}
	var status string
	st.DB.QueryRow("SELECT enrich_status FROM files WHERE id='1'").Scan(&status)
	if status != "skipped" {
		t.Errorf("status %q", status)
	}
}

func TestParseVoteImage(t *testing.T) {
	bodies, _ := fakeOpenRouter(t, `Hier: {"ja": "31", "nein": 12.5, "enthaltung": null, "members": [{"name": " Müller ", "vote": "Dafür"}, {"name": "", "vote": "ja"}, {"name": "Schmidt", "vote": "?"}]} fertig`)
	got := ParseVoteImage([]byte{0xff, 0xd8}, "image/jpeg")
	if got == nil {
		t.Fatal("nil tally")
	}
	want := &VoteTally{Ja: intp(31), Nein: intp(12), Members: []store.VoteMember{{Name: "Müller", Vote: "ja"}, {Name: "Schmidt", Vote: "abwesend"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	b := (*bodies)[0]
	content := b["messages"].([]any)[0].(map[string]any)["content"].([]any)
	img := content[1].(map[string]any)["image_url"].(map[string]any)["url"]
	if b["model"] != VisionModel || b["temperature"] != 0.0 || content[0].(map[string]any)["text"] != votePrompt || img != "data:image/jpeg;base64,/9g=" {
		t.Errorf("request body: %v", b)
	}
}

func TestParseVoteImageNoCounters(t *testing.T) {
	bodies, _ := fakeOpenRouter(t, `{"ja": null, "members": []}`)
	if got := ParseVoteImage([]byte("x"), "image/jpeg"); got != nil {
		t.Errorf("got %+v", got)
	}
	if len(*bodies) != 1 {
		t.Errorf("a tally without counters is final, not retried: %d requests", len(*bodies))
	}
}

func TestParseModelJSON(t *testing.T) {
	for in, want := range map[string]any{
		`{"a": 1}`:                 "1",
		"```json\n{\"a\": 2}\n```": "2",
		`Antwort: {"a": 3} Ende`:   "3",
		"```\n[1]\n```":            nil,
	} {
		m, err := parseModelJSON(in)
		var got any
		if err == nil {
			got = string(m["a"].(json.Number))
		}
		if got != want {
			t.Errorf("parseModelJSON(%q) = %v, %v", in, got, err)
		}
	}
}
