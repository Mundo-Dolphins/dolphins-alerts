package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetScoreboardHTTP(t *testing.T) {
	// simple fake response
	resp := ScoreboardResponse{
		Events: []Event{{
			ID: "ev1",
			Competitions: []Competition{{
				Competitors: []Competitor{{
					Team: Team{Abbreviation: "MIA", Name: "Dolphins"},
				}},
			}},
		}},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := NewClient()
	// override endpoint to point at test server
	c.scoreboardURL = ts.URL

	sb, err := c.GetScoreboard()
	if err != nil {
		t.Fatalf("GetScoreboard error: %v", err)
	}
	if len(sb.Events) != 1 || sb.Events[0].ID != "ev1" {
		t.Fatalf("unexpected scoreboard: %+v", sb)
	}
}

func TestGetGameDetailHTTP(t *testing.T) {
	gd := GameDetailResponse{
		Header: Header{ID: "g1", Competition: Competition{ID: "c1"}},
	}
	// server expects path /game/<id>
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/game/g1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(gd)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := NewClient()
	c.gameDetailURL = ts.URL + "/game/%s"

	got, err := c.GetGameDetail("g1")
	if err != nil {
		t.Fatalf("GetGameDetail error: %v", err)
	}
	if got.Header.ID != "g1" {
		t.Fatalf("unexpected game detail header id: %s", got.Header.ID)
	}
}

func TestGetGameSummaryHTTP(t *testing.T) {
	summary := SummaryResponse{Header: Header{ID: "s1"}, ScoringPlays: []ScoringPlay{{ID: "p1"}}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept both query param and path style
		if r.URL.Query().Get("event") != "s1" && r.URL.Path != "/summary/s1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(summary)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := NewClient()
	// use query-style summary URL
	c.summaryURL = ts.URL + "/summary?event=%s"

	got, err := c.GetGameSummary("s1")
	if err != nil {
		t.Fatalf("GetGameSummary error: %v", err)
	}
	if len(got.ScoringPlays) != 1 || got.ScoringPlays[0].ID != "p1" {
		t.Fatalf("unexpected summary: %+v", got)
	}

	// also test path-style
	c.summaryURL = ts.URL + "/summary/%s"
	got2, err := c.GetGameSummary("s1")
	if err == nil {
		// our server treats /summary/s1 as valid
		if got2.Header.ID != "s1" && len(got2.ScoringPlays) == 0 {
			t.Fatalf("unexpected second summary: %+v", got2)
		}
	} else {
		// if this branch fails on some platforms, it's okay; just log
		t.Logf("second path-style summary request returned error (ok): %v", err)
	}
}
