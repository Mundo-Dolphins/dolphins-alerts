package injury

import (
	"context"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"
)

// InjuryProvider fetches the injury report of one team.
//
// This module ships a provider backed by nflverse's open data
// (NFLVerseProvider). Providers for other sources can be plugged in with
// NewFallbackProvider or Monitor.SetProvider; a provider is responsible for
// having the right to use the source it reads.
type InjuryProvider interface {
	Fetch(ctx context.Context, request ReportRequest) (Report, error)
}

// WithDefaults fills the zero fields of the request (Miami, current season,
// regular season, week 1) so providers can rely on them.
func (r ReportRequest) WithDefaults(now time.Time) ReportRequest {
	if r.Team == "" {
		r.Team = TeamMIA
	}
	if r.Season == 0 {
		r.Season = now.Year()
	}
	if r.SeasonType == "" {
		r.SeasonType = SeasonTypeREG
	}
	if r.Week == 0 {
		r.Week = 1
	}
	return r
}

// NewPlayerReport builds a normalized PlayerReport for providers.
func NewPlayerReport(name, position, injury string, practice PracticeStatus, game GameStatus) PlayerReport {
	return player(name, position, injury, practice, game)
}

// SortPlayers orders players by position and then by name, as the renderer expects.
func SortPlayers(players []PlayerReport) { sortPlayers(players) }

func player(name, position, inj string, practice PracticeStatus, game GameStatus) PlayerReport {
	name = cleanText(name)
	position = strings.ToUpper(cleanText(position))
	inj = cleanText(inj)
	if practice == "" {
		practice = PracticeNotListed
	}
	if game == "" {
		game = GameStatusNotListed
	}
	return PlayerReport{
		ID:         strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		Name:       name,
		Position:   position,
		Injury:     inj,
		Practices:  []PracticeEntry{{Day: "latest", Status: practice}},
		GameStatus: game,
	}
}

func cleanText(s string) string {
	tagRe := regexp.MustCompile(`<[^>]*>`)
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	spaceRe := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

func sortPlayers(players []PlayerReport) {
	sort.Slice(players, func(i, j int) bool {
		if players[i].Position != players[j].Position {
			return players[i].Position < players[j].Position
		}
		return players[i].Name < players[j].Name
	})
}

func inferWeek(now time.Time) int {
	season := inferSeason(now)
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		location = now.Location()
	}
	localNow := now.In(location)
	seasonStart := regularSeasonWeekOneStart(season, location)
	currentDate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	startDate := time.Date(seasonStart.Year(), seasonStart.Month(), seasonStart.Day(), 0, 0, 0, 0, time.UTC)
	w := int(currentDate.Sub(startDate)/(7*24*time.Hour)) + 1
	if w < 1 {
		return 1
	}
	if w > 18 {
		return 18
	}
	return w
}

func inferSeason(now time.Time) int {
	if now.Month() <= time.February {
		return now.Year() - 1
	}
	return now.Year()
}

func regularSeasonWeekOneStart(season int, location *time.Location) time.Time {
	septemberFirst := time.Date(season, time.September, 1, 0, 0, 0, 0, location)
	daysUntilMonday := (int(time.Monday) - int(septemberFirst.Weekday()) + 7) % 7
	laborDay := septemberFirst.AddDate(0, 0, daysUntilMonday)
	return laborDay.AddDate(0, 0, 1)
}
