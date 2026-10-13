package espn

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetGameDetailAndSummary_HTTP(t *testing.T) {
	detailJSON := `{"header":{"id":"game1"},"status":{"type":{"name":"STATUS_SCHEDULED"}},"competitions":[]}`
	summaryJSON := `{"header":{"id":"game1"},"scoringPlays":[{"id":"sp1","text":"Touchdown"}]}`

	// Server that returns different payloads based on path
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/game/game1" || r.URL.Path == "/apis/site/v2/sports/football/nfl/scoreboard/game1" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(detailJSON))
			return
		}
		if r.URL.Path == "/summary" || r.URL.Path == "/apis/site/v2/sports/football/nfl/summary" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(summaryJSON))
			return
		}
		// default
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := NewClient()
	// set endpoints to server
	c.gameDetailURL = fmt.Sprintf("%s/game/%%s", ts.URL)
	c.summaryURL = fmt.Sprintf("%s/summary?event=%%s", ts.URL)

	gd, err := c.GetGameDetail("game1")
	if err != nil {
		t.Fatalf("GetGameDetail error: %v", err)
	}
	if gd.Header.ID != "game1" {
		t.Fatalf("unexpected game detail header id: %v", gd.Header.ID)
	}

	sr, err := c.GetGameSummary("game1")
	if err != nil {
		t.Fatalf("GetGameSummary error: %v", err)
	}
	if len(sr.ScoringPlays) != 1 || sr.Header.ID != "game1" {
		t.Fatalf("unexpected summary: %+v", sr)
	}
}
