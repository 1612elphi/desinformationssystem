package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// Bulk enrichment through the headless `claude -p` CLI, which uses the
// subscription credential in ~/.claude instead of an API key.
var (
	ClaudeBin   = envStr("CLAUDE_BIN", filepath.Join(os.Getenv("HOME"), ".local", "bin", "claude"))
	ClaudeModel = envStr("CLAUDE_MODEL", "sonnet")
	// every invocation loads a ~83k-token harness, so documents are batched
	// per call; ~400 output tokens per doc against a 128k output ceiling
	ClaudeBatchDocs   = envInt("BATCH_DOCS", 25)
	ClaudeConcurrency = envInt("ENRICH_CONCURRENCY", 6)
	ClaudeTimeout     = envSeconds("CLAUDE_TIMEOUT", 900)
	// lower than ENRICH_MAXCHARS because batching multiplies it
	ClaudeMaxChars   = envInt("ENRICH_MAXCHARS_BATCH", 6000)
	ClaudeMaxBatches = envInt("MAX_BATCHES", 0)
	ClaudeMaxSeconds = envInt("ENRICH_MAX_SECONDS", 0)
	// consecutive failed batches taken to mean the plan window is spent
	ClaudeFailStreakStop = envInt("FAIL_STREAK_STOP", 4)
)

// PendingEnrichment lists files with text but no enrichment, newest meeting
// first so a run cut short has enriched the most recent material.
func PendingEnrichment(st *store.Store, limit int) ([]string, error) {
	q := `SELECT f.id FROM files f LEFT JOIN meetings m ON m.id = f.meeting_id
		WHERE f.text_status = 'ok' AND f.enrich_status IN ('skipped', 'pending', 'error')
		ORDER BY m.date DESC, f.id DESC`
	if limit > 0 {
		q += " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := st.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
	cmd := exec.CommandContext(ctx, ClaudeBin, "-p", prompt, "--model", ClaudeModel, "--output-format", "json")
	// a directory without a CLAUDE.md keeps the harness prompt small and cacheable
	cmd.Dir = os.TempDir()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// the CLI reports usage-limit and auth errors as JSON on stdout
		return nil, fmt.Errorf("claude: %v: stderr=%q stdout=%q", err, head(stderr.String(), 300), head(string(out), 400))
	}
	var env struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}
	if env.IsError {
		return nil, fmt.Errorf("claude reported error: %s", head(string(out), 300))
	}
	if strings.TrimSpace(env.Result) == "" {
		return nil, errors.New("claude returned an empty result")
	}
	return parseModelJSON(env.Result)
}

func head(s string, n int) string { return s[:min(len(s), n)] }

func claudeBatch(st *store.Store, ids []string) (written, failed int, err error) {
	var docs []store.Row
	var kept []string
	for _, id := range ids {
		d, err := st.GetDocument(id)
		if err != nil {
			return 0, 0, err
		}
		if d != nil {
			docs = append(docs, d)
			kept = append(kept, id)
		}
	}
	if len(docs) == 0 {
		return 0, 0, nil
	}
	result, err := callClaude(batchPrompt(docs))
	if err != nil {
		return 0, 0, err
	}
	for i, id := range kept {
		entry, ok := result[strconv.Itoa(i)].(map[string]any)
		// a skipped document keeps its status so the next run selects it again
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

func markEnrichError(st *store.Store, ids []string) error {
	return st.Write(func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE files SET enrich_status='error' WHERE id=?", id); err != nil {
				return err
			}
		}
		return nil
	})
}

// EnrichClaude works through the enrichment backlog in batches until it is
// drained, maxBatches calls are made, maxSeconds pass, or
// ClaudeFailStreakStop batches fail in a row.
func EnrichClaude(st *store.Store, maxBatches, maxSeconds, batchDocs, concurrency int) (map[string]int, error) {
	if _, err := os.Stat(ClaudeBin); err != nil {
		return nil, fmt.Errorf("claude CLI not found at %s", ClaudeBin)
	}
	batchDocs, concurrency = max(1, batchDocs), max(1, concurrency)
	todo, err := PendingEnrichment(st, maxBatches*batchDocs)
	if err != nil {
		return nil, err
	}
	var batches [][]string
	for i := 0; i < len(todo); i += batchDocs {
		batches = append(batches, todo[i:min(i+batchDocs, len(todo))])
	}
	log.Printf("enrich-claude: backlog %d docs -> %d batches of %d, concurrency %d",
		len(todo), len(batches), batchDocs, concurrency)

	type res struct {
		ids          []string
		written, bad int
		err          error
	}
	done := make(chan res)
	started := time.Now()
	next, inflight := 0, 0
	submit := func() {
		if next < len(batches) {
			b := batches[next]
			next++
			inflight++
			go func() {
				w, f, err := claudeBatch(st, b)
				done <- res{b, w, f, err}
			}()
		}
	}
	// top the pool up one at a time so an early stop leaves nothing queued
	for range concurrency {
		submit()
	}
	totals := map[string]int{"written": 0, "failed": 0, "batches_ok": 0, "batches_failed": 0}
	streak, stopping := 0, false
	for inflight > 0 {
		r := <-done
		inflight--
		if r.err != nil {
			log.Printf("enrich-claude: batch of %d failed: %v", len(r.ids), r.err)
			if err := markEnrichError(st, r.ids); err != nil {
				log.Printf("enrich-claude: %v", err)
			}
			totals["batches_failed"]++
			streak++
		} else {
			totals["written"] += r.written
			totals["failed"] += r.bad
			totals["batches_ok"]++
			streak = 0
		}
		log.Printf("enrich-claude:   written=%d failed=%d ok_batches=%d bad_batches=%d",
			totals["written"], totals["failed"], totals["batches_ok"], totals["batches_failed"])
		switch {
		case stopping:
		case streak >= ClaudeFailStreakStop:
			log.Printf("enrich-claude: %d batches failed in a row, assuming the plan window is spent; stopping", streak)
			stopping = true
		case maxSeconds > 0 && time.Since(started) > time.Duration(maxSeconds)*time.Second:
			log.Print("enrich-claude: time budget reached")
			stopping = true
		default:
			submit()
		}
	}
	remaining, err := PendingEnrichment(st, 0)
	totals["remaining"] = len(remaining)
	return totals, err
}
