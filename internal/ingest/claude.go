package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var (
	ClaudeBin   = envStr("CLAUDE_BIN", filepath.Join(os.Getenv("HOME"), ".local", "bin", "claude"))
	ClaudeModel = envStr("CLAUDE_MODEL", "sonnet")
	// ~83k-token harness per call
	ClaudeBatchDocs   = envInt("BATCH_DOCS", 25)
	ClaudeConcurrency = envInt("ENRICH_CONCURRENCY", 6)
	ClaudeTimeout     = envSeconds("CLAUDE_TIMEOUT", 900)
	// batching multiplies prompt size
	ClaudeMaxChars   = envInt("ENRICH_MAXCHARS_BATCH", 6000)
	ClaudeMaxBatches = envInt("MAX_BATCHES", 0)
	ClaudeMaxSeconds = envInt("ENRICH_MAX_SECONDS", 0)
	// signals spent plan window
	ClaudeFailStreakStop = envInt("FAIL_STREAK_STOP", 4)
)

const pendingEnrichmentWhere = `WHERE f.text_status = 'ok' AND f.enrich_status IN ('skipped', 'pending', 'error')`

// PendingEnrichment lists files with text but no enrichment, newest meeting
// first so a run cut short has enriched the most recent material.
func PendingEnrichment(st *store.Store, limit int) ([]string, error) {
	q := "SELECT f.id FROM files f LEFT JOIN meetings m ON m.id = f.meeting_id " +
		pendingEnrichmentWhere + " ORDER BY m.date DESC, f.id DESC"
	if limit > 0 {
		q += " LIMIT " + strconv.Itoa(limit)
	}
	return queryStrings(st, q)
}

func PendingEnrichmentCount(st *store.Store) (int, error) {
	var n int
	err := st.DB.QueryRow("SELECT COUNT(*) FROM files f " + pendingEnrichmentWhere).Scan(&n)
	return n, err
}

func batchPrompt(docs []store.Row) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Du bekommst %d Dokumente aus dem Karlsruher Ratsinformationssystem.\n", len(docs))
	b.WriteString("Analysiere JEDES Dokument einzeln.\n\n")
	for i, d := range docs {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "--- DOKUMENT %d ---\n", i)
		b.WriteString("Gremium: " + docStr(d, "body_name", "unbekannt") + "\n")
		b.WriteString("Sitzung: " + docStr(d, "meeting_title", "") + " (" + docStr(d, "meeting_date", "") + ")\n")
		b.WriteString("Dokumenttyp: " + docStr(d, "doc_type", "") + "\n")
		b.WriteString("Bezeichnung: " + docStr(d, "label", "") + "\n")
		b.WriteString("Text:\n\"\"\"\n" + runeHead(docStr(d, "fulltext", ""), ClaudeMaxChars) + "\n\"\"\"\n")
	}
	b.WriteString("\nAntworte mit EINEM JSON-Objekt, ohne Markdown-Codeblock, ohne weiteren Text.\n")
	fmt.Fprintf(&b, "Schlüssel sind die Dokumentnummern als Strings (\"0\" bis \"%d\").\n", len(docs)-1)
	b.WriteString("Jeder Wert ist ein Objekt mit genau diesen Feldern:\n" +
		"  summary_de: 2-4 Sätze deutsche Zusammenfassung des Inhalts.\n" +
		"  summary_en: dieselbe Zusammenfassung auf Englisch.\n" +
		"  category: GENAU einer dieser Werte: " + strings.Join(Categories, ", ") + ".\n" +
		"  topics: Liste von 3-7 kurzen deutschen Schlagworten.\n" +
		"  entities: Objekt mit den Listen people, orgs, locations, je 0-8 Einträge.\n" +
		"Wenn ein Text leer oder unbrauchbar ist: leere Zusammenfassungen und category \"Sonstiges\".\n" +
		"Gib ein Objekt für jede Nummer zurück, auch bei unbrauchbarem Text.")
	return b.String()
}

func callClaude(prompt string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ClaudeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ClaudeBin, "-p", "--model", ClaudeModel, "--output-format", "json")
	// argv exceeds MAX_ARG_STRLEN
	cmd.Stdin = strings.NewReader(prompt)
	// no CLAUDE.md here
	cmd.Dir = os.TempDir()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// errors arrive on stdout
		return nil, fmt.Errorf("claude: %v: stderr=%q stdout=%q", err, runeHead(stderr.String(), 300), runeHead(string(out), 400))
	}
	var env struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}
	if env.IsError {
		return nil, fmt.Errorf("claude reported error: %s", runeHead(string(out), 300))
	}
	if strings.TrimSpace(env.Result) == "" {
		return nil, errors.New("claude returned an empty result")
	}
	return parseModelJSON(env.Result)
}

type ClaudeTotals struct {
	Written, Failed, BatchesOK, BatchesFailed, Remaining int
}

func claudeBatch(st *store.Store, ids []string) (written, failed int, err error) {
	var docs []store.Row
	for _, id := range ids {
		d, err := st.GetDocument(id)
		if err != nil {
			return 0, 0, err
		}
		if d != nil {
			docs = append(docs, d)
		}
	}
	if len(docs) == 0 {
		return 0, 0, nil
	}
	result, err := callClaude(batchPrompt(docs))
	if err != nil {
		return 0, 0, err
	}
	for i, d := range docs {
		id := str(d["id"])
		entry, ok := result[strconv.Itoa(i)].(map[string]any)
		// keep status for retry
		if !ok {
			failed++
			continue
		}
		if err := writeEnrichment(st, id, normaliseEnrichment(entry)); err != nil {
			log.Printf("enrich-claude: write failed for %s: %v", id, err)
			failed++
			continue
		}
		written++
	}
	return written, failed, nil
}

// EnrichClaude works through the enrichment backlog in batches until it is
// drained, maxBatches calls are made, maxSeconds pass, or
// ClaudeFailStreakStop batches fail in a row.
func EnrichClaude(st *store.Store, maxBatches, maxSeconds, batchDocs, concurrency int) (ClaudeTotals, error) {
	var t ClaudeTotals
	if _, err := os.Stat(ClaudeBin); err != nil {
		return t, fmt.Errorf("claude CLI not found at %s", ClaudeBin)
	}
	batchDocs, concurrency = max(1, batchDocs), max(1, concurrency)
	todo, err := PendingEnrichment(st, maxBatches*batchDocs)
	if err != nil {
		return t, err
	}
	batches := make(chan []string, len(todo)/batchDocs+1)
	for i := 0; i < len(todo); i += batchDocs {
		batches <- todo[i:min(i+batchDocs, len(todo))]
	}
	close(batches)
	log.Printf("enrich-claude: backlog %d docs -> %d batches of %d, concurrency %d",
		len(todo), len(batches), batchDocs, concurrency)

	deadline := time.Now().Add(time.Duration(maxSeconds) * time.Second)
	var mu sync.Mutex
	var wg sync.WaitGroup
	streak, stop := 0, false
	for range concurrency {
		wg.Go(func() {
			for b := range batches {
				mu.Lock()
				halt := stop
				mu.Unlock()
				if halt {
					return
				}
				w, f, err := claudeBatch(st, b)
				if err != nil {
					log.Printf("enrich-claude: batch of %d failed: %v", len(b), err)
					if err := setEnrichStatus(st, "error", b...); err != nil {
						log.Printf("enrich-claude: %v", err)
					}
				}
				mu.Lock()
				if err != nil {
					t.BatchesFailed++
					streak++
				} else {
					t.Written += w
					t.Failed += f
					t.BatchesOK++
					streak = 0
				}
				log.Printf("enrich-claude:   written=%d failed=%d ok_batches=%d bad_batches=%d",
					t.Written, t.Failed, t.BatchesOK, t.BatchesFailed)
				if !stop && streak >= ClaudeFailStreakStop {
					log.Printf("enrich-claude: %d batches failed in a row, assuming the plan window is spent; stopping", streak)
					stop = true
				}
				if !stop && maxSeconds > 0 && time.Now().After(deadline) {
					log.Print("enrich-claude: time budget reached")
					stop = true
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	t.Remaining, err = PendingEnrichmentCount(st)
	return t, err
}
