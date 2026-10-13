package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdateCacheAndHelpers(t *testing.T) {
	// prepare a scoreboard event with today's date (RFC3339) and Dolphins competitor
	now := time.Now().UTC()
	event := Event{
		ID:   "game1",
		Date: now.Format(time.RFC3339),
		Competitions: []Competition{{
			Competitors: []Competitor{{
				Team:     Team{Abbreviation: "MIA", Name: "Dolphins"},
				HomeAway: "home",
			},
				{Team: Team{Abbreviation: "NYG", Name: "Giants"}, HomeAway: "away"}},
			Weather: Weather{DisplayValue: "Sunny"},
			Venue:   Venue{FullName: "Test Stadium"},
		}},
		Status: Status{Type: StatusType{Name: StatusScheduled}},
	}

	sb := ScoreboardResponse{Events: []Event{event}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return the same scoreboard/summary/game detail regardless of path for test simplicity
		b, _ := json.Marshal(sb)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := NewClient()
	// point all endpoints at test server
	c.scoreboardURL = ts.URL
	c.gameDetailURL = ts.URL + "/game/%s"
	c.summaryURL = ts.URL + "/summary?event=%s"

	if err := c.UpdateCache(); err != nil {
		t.Fatalf("UpdateCache error: %v", err)
	}

	// cache should contain our game
	if _, ok := c.cache.Games["game1"]; !ok {
		t.Fatalf("expected game1 in cache, cache=%+v", c.cache.Games)
	}

	// Test marking scoring play and checking seen
	c.MarkScoringPlayAsSeen("game1", "play1")
	if !c.IsScoringPlaySeen("game1", "play1") {
		t.Fatalf("IsScoringPlaySeen expected true after marking")
	}

	// UpdateGameStatus should update status and IsGameActive field
	c.UpdateGameStatus("game1", Status{Type: StatusType{Name: StatusInProgress}})
	g := c.cache.Games["game1"]
	if !g.IsActive {
		t.Fatalf("expected game1 to be active after UpdateGameStatus")
	}

	// GetActiveGames should include the game
	active := c.GetActiveGames()
	if len(active) == 0 {
		t.Fatalf("GetActiveGames expected non-empty")
	}

	// ShouldCacheBeUpdated: force LastUpdated to zero and test true
	c.cache.LastUpdated = time.Time{}
	if !c.ShouldCacheBeUpdated() {
		t.Fatalf("ShouldCacheBeUpdated expected true when LastUpdated zero")
	}
}
