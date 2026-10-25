// Package typesafe is a minimal client for TypeSafe (Jev / System One):
// typed decisions with calibrated confidence. See https://docs.typesafe.ai/api.
//
// TYPESAFE_BASE_URL may point at the switchboard proxy, which injects the key;
// TYPESAFE_API_KEY is needed only when calling api.typesafe.ai directly.
package typesafe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	BaseURL  = strings.TrimRight(env("TYPESAFE_BASE_URL", "https://api.typesafe.ai"), "/")
	Endpoint = BaseURL + "/v1/systemone"
	Model    = env("TYPESAFE_MODEL", "jev-latest")

	client = &http.Client{Timeout: 60 * time.Second}
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Available reports whether a call can authenticate: a proxy base
// authenticates upstream, direct calls need a key.
func Available() bool {
	return os.Getenv("TYPESAFE_BASE_URL") != "" || os.Getenv("TYPESAFE_API_KEY") != ""
}

// Criterion is one option of a choice question; a nil Desc is sent as null.
type Criterion struct {
	Key  string
	Desc *string
}

// Criteria marshals as a JSON object in slice order, since Python sends the
// options in dict insertion order.
type Criteria []Criterion

func (c Criteria) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, cr := range c {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(cr.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(cr.Desc)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

type Question struct {
	Type         string   `json:"type"`
	Instructions string   `json:"instructions"`
	Criteria     Criteria `json:"criteria"`
}

type Answer struct {
	Choice     string  `json:"choice"`
	Confidence float64 `json:"confidence"`
	// choice -> probability, verbatim from the response
	Probabilities json.RawMessage `json:"probabilities"`
}

func Choice(instructions string, criteria Criteria) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: criteria}
}

// StatusError is a non-2xx response that was not retried.
type StatusError struct{ Code int }

func (e StatusError) Error() string { return fmt.Sprintf("typesafe: HTTP %d", e.Code) }

// SystemOne evaluates questions against state and returns the answer per
// question id, retrying 429/529 and network errors with backoff.
func SystemOne(state any, questions map[string]Question, retries int) (map[string]Answer, error) {
	body, err := json.Marshal(struct {
		Model     string              `json:"model"`
		State     any                 `json:"state"`
		Questions map[string]Question `json:"questions"`
	}{Model, state, questions})
	if err != nil {
		return nil, err
	}
	for attempt := range retries {
		backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		req, err := http.NewRequest(http.MethodPost, Endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if key := os.Getenv("TYPESAFE_API_KEY"); key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := client.Do(req)
		if err != nil {
			if attempt == retries-1 {
				return nil, err
			}
			time.Sleep(backoff)
			continue
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			if (resp.StatusCode == 429 || resp.StatusCode == 529) && attempt < retries-1 {
				if ra := resp.Header.Get("retry-after"); ra != "" {
					secs, err := strconv.ParseFloat(ra, 64)
					if err != nil {
						return nil, err
					}
					backoff = time.Duration(secs * float64(time.Second))
				}
				time.Sleep(backoff)
				continue
			}
			return nil, StatusError{resp.StatusCode}
		}
		var out struct {
			Answers map[string]Answer `json:"answers"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if out.Answers == nil {
			out.Answers = map[string]Answer{}
		}
		return out.Answers, nil
	}
	return map[string]Answer{}, nil
}
