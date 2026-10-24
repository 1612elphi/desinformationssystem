// Package ingest is the HTML scraper, the shared file pipeline, the
// OpenRouter enrichment and vote parsers, and the live-ticker watcher.
package ingest

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return n
	}
	return def
}

func envSeconds(key string, def float64) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64)
	if err != nil {
		f = def
	}
	return time.Duration(f * float64(time.Second))
}

var (
	UserAgent       = envStr("USER_AGENT", "DesinfoBot/1.0 (+personal Karlsruhe Sitzungskalender archive; contact rmv@rmv.fyi)")
	RequestInterval = envSeconds("REQUEST_INTERVAL", 1.5)
	MaxRetries      = envInt("MAX_RETRIES", 5)
	FlareSolverrURL = envStr("FLARESOLVERR_URL", "")
)

// Response is a fully read HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r *Response) Text() string { return string(r.Body) }

// Fetcher is the polite, throttled, retrying HTTP client of the ingesters.
// It is not safe for concurrent use.
type Fetcher struct {
	client *http.Client
	last   time.Time
}

func NewFetcher() *Fetcher {
	return &Fetcher{client: &http.Client{Timeout: 60 * time.Second}}
}

func (f *Fetcher) Close() { f.client.CloseIdleConnections() }

func (f *Fetcher) throttle() {
	if wait := RequestInterval - time.Since(f.last); wait > 0 {
		time.Sleep(wait)
	}
	f.last = time.Now()
}

func (f *Fetcher) do(method, url string) (*Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{resp.StatusCode, resp.Header, body}, nil
}

// retryAfter prefers the server's delta-seconds Retry-After (the only form
// web1 has been seen to send), capped at 300 s.
func retryAfter(h http.Header, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(h.Get("Retry-After")); raw != "" {
		if s, err := strconv.ParseFloat(raw, 64); err == nil && !math.IsNaN(s) {
			return time.Duration(max(0, min(300, s)) * float64(time.Second))
		}
	}
	return fallback
}

func pow2(n int) time.Duration { return time.Duration(1<<n) * time.Second }

// Get retries with backoff and returns nil on persistent failure. Without
// binary, a 200 must be HTML or JSON.
func (f *Fetcher) Get(url string, binary bool) *Response {
	for attempt := range MaxRetries {
		f.throttle()
		r, err := f.do(http.MethodGet, url)
		if err != nil {
			log.Printf("filestore: request error %s (%v) attempt %d", url, err, attempt+1)
			if attempt < MaxRetries-1 {
				time.Sleep(pow2(attempt))
			}
			continue
		}
		ct := r.Header.Get("Content-Type")
		// web1 returns a 404 page when rate-limited, so 404/429/5xx are retried
		if r.Status == 200 && (binary || strings.Contains(ct, "text/html") || strings.Contains(ct, "json")) {
			return r
		}
		if r.Status == 403 && FlareSolverrURL != "" && !binary {
			if fs := f.flaresolverr(url); fs != nil {
				return fs
			}
		}
		log.Printf("filestore: bad status %d for %s (attempt %d/%d)", r.Status, url, attempt+1, MaxRetries)
		if attempt < MaxRetries-1 {
			time.Sleep(retryAfter(r.Header, min(60*time.Second, 3*pow2(attempt))))
		}
	}
	log.Printf("filestore: giving up on %s", url)
	return nil
}

// Head is a cheap change check (Last-Modified / Content-Length).
func (f *Fetcher) Head(url string) *Response {
	for attempt := range 2 {
		f.throttle()
		if r, err := f.do(http.MethodHead, url); err == nil && r.Status == 200 {
			return r
		}
		time.Sleep(pow2(attempt))
	}
	return nil
}

func (f *Fetcher) flaresolverr(url string) *Response {
	body, _ := json.Marshal(map[string]any{"cmd": "request.get", "url": url, "maxTimeout": 60000})
	c := &http.Client{Timeout: 90 * time.Second}
	resp, err := c.Post(FlareSolverrURL+"/v1", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("filestore: flaresolverr failed for %s: %v", url, err)
		return nil
	}
	defer resp.Body.Close()
	var out struct {
		Solution struct {
			Status   int    `json:"status"`
			Response string `json:"response"`
		} `json:"solution"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Printf("filestore: flaresolverr failed for %s: %v", url, err)
		return nil
	}
	if out.Solution.Status == 200 {
		return &Response{200, http.Header{}, []byte(out.Solution.Response)}
	}
	return nil
}
