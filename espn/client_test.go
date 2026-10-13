package espn

import (
	"testing"
	"time"
)

func TestFindDolphinsGames(t *testing.T) {
	c := NewClient()

	ev := Event{
		ID:   "1",
		Date: time.Now().Format(time.RFC3339),
		Competitions: []Competition{
			{
				Competitors: []Competitor{
					{Team: Team{Abbreviation: DolphinsAbbreviation, Name: "Miami Dolphins"}},
				},
			},
		},
	}

	sb := &ScoreboardResponse{Events: []Event{ev}}
	res := c.FindDolphinsGames(sb)
	if len(res) != 1 {
		t.Fatalf("expected 1 dolphins game, got %d", len(res))
	}
}

func TestGameStatusHelpers(t *testing.T) {
	c := NewClient()

	if !c.IsGameActive(Status{Type: StatusType{Name: StatusInProgress}}) {
		t.Fatalf("expected active")
	}
	if !c.IsGameScheduled(Status{Type: StatusType{Name: StatusScheduled}}) {
		t.Fatalf("expected scheduled")
	}
	if !c.IsGameFinished(Status{Type: StatusType{Name: StatusFinal}}) {
		t.Fatalf("expected finished")
	}
}

func TestCacheAndScoringPlays(t *testing.T) {
	c := NewClient()

	// Cache empty -> ShouldCacheBeUpdated true
	c.cache.LastUpdated = time.Time{}
	if !c.ShouldCacheBeUpdated() {
		t.Fatalf("expected ShouldCacheBeUpdated when LastUpdated zero")
	}

	// Populate cache with a game
	gid := "game1"
	g := GameCache{ID: gid, SeenScoringPlays: make(map[string]bool)}
	c.cache.Games[gid] = g

	if c.IsScoringPlaySeen(gid, "play1") {
		t.Fatalf("expected scoring play not seen")
	}

	c.MarkScoringPlayAsSeen(gid, "play1")
	if !c.IsScoringPlaySeen(gid, "play1") {
		t.Fatalf("expected scoring play seen after marking")
	}

	// Update game status
	c.UpdateGameStatus(gid, Status{Type: StatusType{Name: StatusInProgress}})
	if !c.cache.Games[gid].IsActive {
		t.Fatalf("expected game to be active after update")
	}
}
