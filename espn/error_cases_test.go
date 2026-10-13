package espn

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetScoreboard_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient()
	c.scoreboardURL = ts.URL

	if _, err := c.GetScoreboard(); err == nil {
		t.Fatalf("expected error for non-200 scoreboard response")
	}
}

func TestGetGameDetail_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := NewClient()
	c.gameDetailURL = ts.URL + "/game/%s"

	if _, err := c.GetGameDetail("x"); err == nil {
		t.Fatalf("expected error when game detail returns non-200")
	}
}

func TestGetGameSummary_BadJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return invalid json
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{invalid-json}"))
	}))
	defer ts.Close()

	c := NewClient()
	c.summaryURL = ts.URL + "/summary?event=%s"

	if _, err := c.GetGameSummary("s"); err == nil {
		t.Fatalf("expected unmarshal error for bad JSON summary")
	}
}
