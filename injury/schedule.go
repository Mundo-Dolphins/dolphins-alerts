package injury

import (
	"context"
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/Mundo-Dolphins/dolphins-alerts/espn"
)

// ESPN keeps its own in-memory cache; never read or rewrite the game monitor's
// persisted state. Refresh at most every six hours, retrying on errors.
type ESPNScheduleProvider struct {
	client      espn.ClientAPI
	lastRefresh time.Time
	now         func() time.Time
}

func (p *ESPNScheduleProvider) NextGame(ctx context.Context, team TeamCode) (Game, error) {
	if err := ctx.Err(); err != nil {
		return Game{}, err
	}
	if team != TeamMIA {
		return Game{}, fmt.Errorf("unsupported schedule team: %s", team)
	}
	now := time.Now()
	if p.now != nil {
		now = p.now()
	}
	if p.lastRefresh.IsZero() || now.Sub(p.lastRefresh) >= 6*time.Hour {
		if err := p.client.UpdateCache(); err != nil {
			return Game{}, err
		}
		p.lastRefresh = now
	}
	var next Game
	for _, game := range p.client.GetActiveGames() {
		if game.Date.After(now) && (next.Start.IsZero() || game.Date.Before(next.Start)) {
			seasonType := SeasonTypeREG
			if game.Season.Type == 3 {
				seasonType = SeasonTypePOST
			}
			next = Game{Team: team, Start: game.Date, Season: game.Season.Year, SeasonType: seasonType, Week: game.Week.Number}
			for _, competitor := range []*espn.Competitor{game.HomeTeam, game.AwayTeam} {
				if competitor != nil && competitor.Team.Abbreviation != "MIA" && competitor.Team.DisplayName != "Miami Dolphins" {
					next.Opponent = TeamCode(competitor.Team.Abbreviation)
					next.OpponentName = competitor.Team.DisplayName
				}
			}
		}
	}
	return next, nil
}

func (m *Monitor) currentTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// Classify the game by its US date, then poll on the corresponding calendar
// dates in Madrid. Prime-time Sunday games can kick off on Monday in Spain.
func publishWindowStarts(game Game) []time.Time {
	if game.Start.IsZero() {
		return nil
	}
	eastern, _ := time.LoadLocation("America/New_York")
	madrid, _ := time.LoadLocation("Europe/Madrid")
	date := game.Start.In(eastern)
	var offsets []int
	switch date.Weekday() {
	case time.Sunday, time.Monday, time.Saturday:
		offsets = []int{-4, -3, -2}
	case time.Thursday, time.Friday, time.Wednesday:
		offsets = []int{-3, -2, -1}
	default:
		return nil
	}
	base := time.Date(date.Year(), date.Month(), date.Day(), 22, 0, 0, 0, madrid)
	starts := make([]time.Time, 0, len(offsets))
	for _, offset := range offsets {
		starts = append(starts, base.AddDate(0, 0, offset))
	}
	return starts
}

func inPublishWindow(now time.Time, game Game) bool {
	if !now.Before(game.Start) {
		return false
	}
	for _, start := range publishWindowStarts(game) {
		end := time.Date(start.Year(), start.Month(), start.Day()+1, 2, 0, 0, 0, start.Location())
		if !now.Before(start) && now.Before(end) {
			return true
		}
	}
	return false
}
