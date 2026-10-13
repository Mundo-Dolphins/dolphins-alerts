package injury

import (
	"fmt"
	"testing"
	"time"

	"github.com/Mundo-Dolphins/dolphins-alerts/espn"
)

// fakeESPNClient implements espn.ClientAPI for testing
type fakeESPNClient struct {
	games    []espn.GameCache
	updError error
}

func (f *fakeESPNClient) UpdateCache() error               { return f.updError }
func (f *fakeESPNClient) GetActiveGames() []espn.GameCache { return f.games }

// helper to compute next weekday from now in CET
func nextWeekday(from time.Time, weekday time.Weekday, loc *time.Location) time.Time {
	// normalize to given location
	now := from.In(loc)
	daysUntil := (int(weekday) - int(now.Weekday()) + 7) % 7
	if daysUntil == 0 {
		daysUntil = 7
	}
	next := now.AddDate(0, 0, daysUntil)
	return time.Date(next.Year(), next.Month(), next.Day(), 20, 0, 0, 0, loc)
}

func TestIsNextDolphinsGameOnThursday_ReturnsTrue(t *testing.T) {
	cet, _ := time.LoadLocation("Europe/Berlin")
	now := time.Now()
	nextThu := nextWeekday(now, time.Thursday, cet)

	fake := &fakeESPNClient{
		games: []espn.GameCache{{
			ID:   "g1",
			Date: nextThu,
		}},
	}

	m := &Monitor{espnClient: fake}

	ok, err := m.isNextDolphinsGameOnThursday()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected next game to be Thursday, got false")
	}
}

func TestIsNextDolphinsGameOnThursday_ReturnsFalseWhenNotThursday(t *testing.T) {
	cet, _ := time.LoadLocation("Europe/Berlin")
	now := time.Now()
	nextMon := nextWeekday(now, time.Monday, cet)

	fake := &fakeESPNClient{
		games: []espn.GameCache{{
			ID:   "g2",
			Date: nextMon,
		}},
	}

	m := &Monitor{espnClient: fake}

	ok, err := m.isNextDolphinsGameOnThursday()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected next game NOT to be Thursday, got true")
	}
}

func TestIsNextDolphinsGameOnThursday_PropagatesError(t *testing.T) {
	fake := &fakeESPNClient{updError: fmt.Errorf("network")}
	m := &Monitor{espnClient: fake}
	_, err := m.isNextDolphinsGameOnThursday()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
