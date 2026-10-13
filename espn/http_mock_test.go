package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetScoreboardForDate_HTTP(t *testing.T) {
	// create a minimal scoreboard response for a specific date
	resp := ScoreboardResponse{
		Events: []Event{{
			ID: "d1",
			Competitions: []Competition{{
				Competitors: []Competitor{{
					Team: Team{Abbreviation: "MIA", Name: "Miami Dolphins"},
				}},
			}},
		}},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ensure the date query param exists
		if r.URL.Query().Get("dates") == "20250101" || r.URL.Query().Get("dates") != "" {
			b, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(b)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := NewClient()
	c.scoreboardURL = ts.URL

	got, err := c.GetScoreboardForDate("20250101")
	if err != nil {
		t.Fatalf("GetScoreboardForDate error: %v", err)
	}
	if len(got.Events) != 1 || got.Events[0].ID != "d1" {
		t.Fatalf("unexpected scoreboard for date: %+v", got)
	}
}

func TestFindDolphinsGamesAndStatusHelpers(t *testing.T) {
	// Build a scoreboard with mixed teams
	sb := &ScoreboardResponse{
		Events: []Event{
			{ID: "e1", Competitions: []Competition{{Competitors: []Competitor{{Team: Team{Abbreviation: "MIA", Name: "Dolphins"}}}}}},
			{ID: "e2", Competitions: []Competition{{Competitors: []Competitor{{Team: Team{Abbreviation: "NYG", Name: "Giants"}}}}}},
		},
	}

	c := NewClient()
	games := c.FindDolphinsGames(sb)
	if len(games) != 1 || games[0].ID != "e1" {
		t.Fatalf("FindDolphinsGames returned unexpected: %+v", games)
	}

	// Test status helpers
	if !c.IsGameScheduled(Status{Type: StatusType{Name: StatusScheduled}}) {
		t.Fatalf("IsGameScheduled should return true for scheduled status")
	}
	if !c.IsGameActive(Status{Type: StatusType{Name: StatusInProgress}}) {
		t.Fatalf("IsGameActive should return true for in-progress status")
	}
	if !c.IsGameFinished(Status{Type: StatusType{Name: StatusFinal}}) {
		t.Fatalf("IsGameFinished should return true for final status")
	}
}
