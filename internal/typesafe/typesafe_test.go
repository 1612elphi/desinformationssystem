package typesafe

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemOneRetriesAndAuth(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization %q", got)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type %q", r.Header.Get("Content-Type"))
		}
		switch calls {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		case 2:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(529)
		default:
			w.Write([]byte(`{"answers":{"q":{"choice":"a","confidence":0.75}}}`))
		}
	}))
	defer srv.Close()
	Endpoint = srv.URL
	t.Setenv("TYPESAFE_API_KEY", "k")

	ans, err := SystemOne("s", map[string]Question{"q": Choice("i", Criteria{{Key: "a"}})}, 5)
	if err != nil || calls != 3 || ans["q"] != (Answer{"a", 0.75}) {
		t.Fatalf("got %v %v after %d calls", ans, err, calls)
	}
}

func TestSystemOneFailures(t *testing.T) {
	code := 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(code)
	}))
	defer srv.Close()
	Endpoint = srv.URL
	var se StatusError
	if _, err := SystemOne("s", nil, 5); !errors.As(err, &se) || se.Code != 500 {
		t.Errorf("500: got %v", err)
	}
	code = 429
	if _, err := SystemOne("s", nil, 2); !errors.As(err, &se) || se.Code != 429 {
		t.Errorf("429 on last attempt: got %v", err)
	}
}

func TestAvailable(t *testing.T) {
	t.Setenv("TYPESAFE_BASE_URL", "")
	t.Setenv("TYPESAFE_API_KEY", "")
	if Available() {
		t.Error("available without base url or key")
	}
	t.Setenv("TYPESAFE_BASE_URL", "http://switchboard:3000/typesafe")
	if !Available() {
		t.Error("unavailable with base url")
	}
}
