package espn

import (
	"testing"
	"time"
)

func TestFindDolphinsGames_StatusHelpers(t *testing.T) {
	c := NewClient()

	// Build a scoreboard with two events, one contains Dolphins by abbreviation
	sb := &ScoreboardResponse{
		Events: []Event{
			{
				ID:   "1",
				Date: time.Now().Format(time.RFC3339),
				Competitions: []Competition{
					{
						Competitors: []Competitor{
							{Team: Team{Abbreviation: "MIA", Name: "Miami Dolphins"}},
						},
					},
				},
			},
			{
				ID:   "2",
				Date: time.Now().Format(time.RFC3339),
				Competitions: []Competition{
					{
						Competitors: []Competitor{
							{Team: Team{Abbreviation: "NYJ", Name: "New York Jets"}},
						},
					},
				},
			},
		},
	}

	games := c.FindDolphinsGames(sb)
	if len(games) != 1 {
		t.Fatalf("expected 1 dolphins game, got %d", len(games))
	}

	// Test status helpers
	sScheduled := Status{Type: StatusType{Name: StatusScheduled}}
	sActive := Status{Type: StatusType{Name: StatusInProgress}}
	sFinal := Status{Type: StatusType{Name: StatusFinal}}

	if !c.IsGameScheduled(sScheduled) {
		t.Fatalf("IsGameScheduled should be true for scheduled status")
	}
	if !c.IsGameActive(sActive) {
		t.Fatalf("IsGameActive should be true for in-progress status")
	}
	if !c.IsGameFinished(sFinal) {
		t.Fatalf("IsGameFinished should be true for final status")
	}
}

func TestCacheScoringAndUpdateStatus_Helpers(t *testing.T) {
	c := NewClient()

	// Prepare a cached game
	gc := GameCache{
		ID:               "g1",
		Date:             time.Now(),
		Status:           StatusScheduled,
		LastChecked:      time.Now(),
		IsActive:         false,
		SeenScoringPlays: make(map[string]bool),
	}

	c.cache.Games[gc.ID] = gc

	// Mark scoring play
	if c.IsScoringPlaySeen(gc.ID, "sp1") {
		t.Fatalf("expected scoring play not seen initially")
	}
	c.MarkScoringPlayAsSeen(gc.ID, "sp1")
	if !c.IsScoringPlaySeen(gc.ID, "sp1") {
		t.Fatalf("expected scoring play to be marked seen")
	}

	// Update status to in-progress
	c.UpdateGameStatus(gc.ID, Status{Type: StatusType{Name: StatusInProgress}})
	updated := c.cache.Games[gc.ID]
	if !updated.IsActive {
		t.Fatalf("expected game to be active after UpdateGameStatus")
	}

	// GetActiveGames should return it
	active := c.GetActiveGames()
	if len(active) == 0 {
		t.Fatalf("expected at least one active game after update")
	}
}

func TestShouldCacheBeUpdated(t *testing.T) {
	c := NewClient()
	// Fresh client with zero LastUpdated should return true
	if !c.ShouldCacheBeUpdated() {
		t.Fatalf("expected ShouldCacheBeUpdated true for zero LastUpdated")
	}

	// Set LastUpdated to now -> should be false
	c.cache.LastUpdated = time.Now()
	if c.ShouldCacheBeUpdated() {
		t.Fatalf("expected ShouldCacheBeUpdated false for recently updated cache")
	}

	// Set LastUpdated far in the past -> should be true
	c.cache.LastUpdated = time.Now().Add(-48 * time.Hour)
	if !c.ShouldCacheBeUpdated() {
		t.Fatalf("expected ShouldCacheBeUpdated true for stale cache")
	}
}
