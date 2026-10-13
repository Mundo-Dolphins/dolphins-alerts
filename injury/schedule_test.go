package injury

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Mundo-Dolphins/dolphins-alerts/espn"
)

func scheduleTime(t *testing.T, value string) time.Time {
	t.Helper()
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublishWindow(t *testing.T) {
	for _, tc := range []struct {
		name, game, now string
		want            bool
	}{
		{"Monday regression", "2026-10-11T20:25:00Z", "2026-10-05T22:30:00+02:00", false},
		{"Tuesday regression", "2026-10-11T20:25:00Z", "2026-10-06T22:30:00+02:00", false},
		{"before opening", "2026-10-11T20:25:00Z", "2026-10-07T21:59:59+02:00", false},
		{"opening", "2026-10-11T20:25:00Z", "2026-10-07T22:00:00+02:00", true},
		{"Thursday", "2026-10-11T20:25:00Z", "2026-10-08T23:59:59+02:00", true},
		{"Friday", "2026-10-11T20:25:00Z", "2026-10-09T22:00:00+02:00", true},
		{"midnight still active", "2026-10-11T20:25:00Z", "2026-10-10T00:00:00+02:00", true},
		{"late Friday report", "2026-10-11T20:25:00Z", "2026-10-10T01:59:59+02:00", true},
		{"closing at 02", "2026-10-11T20:25:00Z", "2026-10-10T02:00:00+02:00", false},
		{"before first window", "2026-10-11T20:25:00Z", "2026-10-07T01:00:00+02:00", false},
		{"game day", "2026-10-11T20:25:00Z", "2026-10-11T22:30:00+02:00", false},
		{"Thursday game Monday", "2026-10-09T00:15:00Z", "2026-10-05T22:00:00+02:00", true},
		{"Thursday game Wednesday", "2026-10-09T00:15:00Z", "2026-10-07T22:00:00+02:00", true},
		{"Thursday game Friday", "2026-10-09T00:15:00Z", "2026-10-09T22:00:00+02:00", false},
		{"Monday game Saturday", "2026-10-13T00:15:00Z", "2026-10-10T22:00:00+02:00", true},
		{"Sunday night US date", "2026-10-12T00:20:00Z", "2026-10-07T22:00:00+02:00", true},
		{"Sunday night Saturday blocked", "2026-10-12T00:20:00Z", "2026-10-10T22:00:00+02:00", false},
		{"bye week", "2026-10-18T17:00:00Z", "2026-10-07T22:00:00+02:00", false},
		{"Madrid winter time", "2026-11-08T18:00:00Z", "2026-11-04T22:00:00+01:00", true},
		{"Friday game", "2026-12-25T18:00:00Z", "2026-12-22T22:00:00+01:00", true},
		{"Saturday game", "2026-12-26T18:00:00Z", "2026-12-24T22:00:00+01:00", true},
		{"Wednesday game", "2026-12-23T18:00:00Z", "2026-12-20T22:00:00+01:00", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inPublishWindow(scheduleTime(t, tc.now), Game{Start: scheduleTime(t, tc.game)}); got != tc.want {
				t.Fatalf("window = %v, want %v", got, tc.want)
			}
		})
	}
	if inPublishWindow(time.Now(), Game{}) {
		t.Fatal("empty schedule must block publication")
	}
}

type fixedSchedule struct {
	game Game
	err  error
}

func (s fixedSchedule) NextGame(context.Context, TeamCode) (Game, error) { return s.game, s.err }

func TestMonitorScheduleGate(t *testing.T) {
	for _, tc := range []struct {
		name, now             string
		scheduleErr           error
		force, dryRun         bool
		fetches, publications int
	}{
		{name: "Monday stale report", now: "2026-10-05T22:30:00+02:00"},
		{name: "before 22", now: "2026-10-07T21:59:00+02:00"},
		{name: "active", now: "2026-10-07T22:00:00+02:00", fetches: 1, publications: 1},
		{name: "late Friday report", now: "2026-10-10T01:45:00+02:00", fetches: 1, publications: 1},
		{name: "closed at 02", now: "2026-10-10T02:00:00+02:00"},
		{name: "ESPN unavailable", now: "2026-10-07T22:00:00+02:00", scheduleErr: errors.New("offline")},
		{name: "manual force", now: "2026-10-05T22:30:00+02:00", force: true, fetches: 1, publications: 1},
		{name: "dry run", now: "2026-10-05T22:30:00+02:00", dryRun: true, fetches: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/injury.json"
			m := NewMonitor(path, "", "")
			m.now = func() time.Time { return scheduleTime(t, tc.now) }
			m.schedule = fixedSchedule{game: Game{Start: scheduleTime(t, "2026-10-11T20:25:00Z")}, err: tc.scheduleErr}
			provider := &sequenceProvider{reports: []Report{testReport("Old report", PracticeDNP)}}
			publisher := &capturePublisher{}
			m.SetProvider(provider)
			m.SetPublisher(publisher)
			_, err := m.Check(context.Background(), tc.force, tc.dryRun)
			if (err != nil) != (tc.scheduleErr != nil) {
				t.Fatalf("unexpected error: %v", err)
			}
			if provider.index != tc.fetches || len(publisher.texts) != tc.publications {
				t.Fatalf("fetches=%d publications=%d", provider.index, len(publisher.texts))
			}
			if tc.publications == 0 {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("blocked/dry check wrote state: %v", err)
				}
			}
		})
	}
}

func TestPollIntervalWakesAtOpening(t *testing.T) {
	m := NewMonitor("", "", "")
	m.schedule = fixedSchedule{game: Game{Start: scheduleTime(t, "2026-10-11T20:25:00Z")}}
	for _, tc := range []struct {
		now  string
		want time.Duration
	}{
		{"2026-10-07T20:30:00+02:00", 90 * time.Minute},
		{"2026-10-07T22:00:00+02:00", 15 * time.Minute},
		{"2026-10-10T01:45:00+02:00", 15 * time.Minute},
		{"2026-10-10T02:00:00+02:00", 6 * time.Hour},
		{"2026-10-05T22:30:00+02:00", 6 * time.Hour},
	} {
		m.now = func() time.Time { return scheduleTime(t, tc.now) }
		if got := m.pollInterval(context.Background()); got != tc.want {
			t.Fatalf("%s: interval=%v want=%v", tc.now, got, tc.want)
		}
	}
}

func TestESPNScheduleSelectsNextGameAndFailsClosed(t *testing.T) {
	now := scheduleTime(t, "2026-10-06T12:00:00Z")
	client := &fakeESPNClient{games: []espn.GameCache{
		{Date: now.Add(12 * 24 * time.Hour)},
		{Date: now.Add(-24 * time.Hour)},
		{Date: now.Add(5 * 24 * time.Hour), Season: espn.ScoreboardSeason{Year: 2026, Type: 2}, Week: espn.ScoreboardWeek{Number: 5}, HomeTeam: &espn.Competitor{Team: espn.Team{Abbreviation: "MIA", DisplayName: "Miami Dolphins"}}, AwayTeam: &espn.Competitor{Team: espn.Team{Abbreviation: "CIN", DisplayName: "Cincinnati Bengals"}}},
	}}
	p := &ESPNScheduleProvider{client: client, now: func() time.Time { return now }}
	game, err := p.NextGame(context.Background(), TeamMIA)
	if err != nil || !game.Start.Equal(now.Add(5*24*time.Hour)) {
		t.Fatalf("next=%+v err=%v", game, err)
	}
	if game.Week != 5 || game.Season != 2026 || game.OpponentName != "Cincinnati Bengals" {
		t.Fatalf("missing game metadata: %+v", game)
	}
	client.updError = errors.New("offline")
	if _, err := p.NextGame(context.Background(), TeamMIA); err != nil {
		t.Fatal("should reuse cached schedule")
	}
	now = now.Add(6 * time.Hour)
	if _, err := p.NextGame(context.Background(), TeamMIA); err == nil {
		t.Fatal("refresh failure must block")
	}
	client.updError = nil
	client.games = nil
	game, err = p.NextGame(context.Background(), TeamMIA)
	if err != nil || !game.Start.IsZero() {
		t.Fatalf("empty schedule: %+v %v", game, err)
	}
}

func TestInjuryDoesNotFetchDuringByeWeek(t *testing.T) {
	path := t.TempDir() + "/injury.json"
	m := NewMonitor(path, "", "")
	m.schedule = fixedSchedule{game: Game{Start: scheduleTime(t, "2026-10-25T17:00:00Z")}}
	provider := &sequenceProvider{reports: []Report{testReport("Friday report", PracticeDNP)}}
	publisher := &capturePublisher{}
	m.SetProvider(provider)
	m.SetPublisher(publisher)
	for _, date := range []string{"2026-10-12T17:00:00+02:00", "2026-10-14T22:00:00+02:00", "2026-10-16T23:00:00+02:00"} {
		m.now = func() time.Time { return scheduleTime(t, date) }
		if _, err := m.Check(context.Background(), false, false); err != nil {
			t.Fatal(err)
		}
	}
	if provider.index != 0 || len(publisher.texts) != 0 {
		t.Fatal("bye week fetched or published the old report")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("bye week modified state")
	}
}

func TestFridayReportIsNotRepeatedAfterMidnightOrRestart(t *testing.T) {
	path := t.TempDir() + "/injury.json"
	for i, date := range []string{"2026-10-09T23:00:00+02:00", "2026-10-10T01:00:00+02:00"} {
		m := NewMonitor(path, "", "")
		m.schedule = fixedSchedule{game: Game{Start: scheduleTime(t, "2026-10-11T17:00:00Z")}}
		m.now = func() time.Time { return scheduleTime(t, date) }
		m.SetProvider(&sequenceProvider{reports: []Report{testReport("Friday report", PracticeDNP)}})
		publisher := &capturePublisher{}
		m.SetPublisher(publisher)
		if _, err := m.Check(context.Background(), false, false); err != nil {
			t.Fatal(err)
		}
		want := 1 - i
		if len(publisher.texts) != want {
			t.Fatalf("date %s: sent %d, want %d", date, len(publisher.texts), want)
		}
	}
}

type requestProvider struct {
	request ReportRequest
	report  Report
}

func (p *requestProvider) Fetch(_ context.Context, request ReportRequest) (Report, error) {
	p.request = request
	return p.report, nil
}

func TestMonitorUsesGameWeekAndRejectsOldOpponent(t *testing.T) {
	path := t.TempDir() + "/injury.json"
	m := NewMonitor(path, "", "")
	m.schedule = fixedSchedule{game: Game{Start: scheduleTime(t, "2026-10-25T17:00:00Z"), Season: 2026, SeasonType: SeasonTypeREG, Week: 8, OpponentName: "New York Jets"}}
	m.now = func() time.Time { return scheduleTime(t, "2026-10-21T22:00:00+02:00") }
	report := testReport("Old player", PracticeDNP)
	report.OpponentName = "Cincinnati Bengals"
	provider := &requestProvider{report: report}
	publisher := &capturePublisher{}
	m.SetProvider(provider)
	m.SetPublisher(publisher)
	if _, err := m.Check(context.Background(), false, false); err != nil {
		t.Fatal(err)
	}
	if provider.request.Week != 8 || provider.request.Season != 2026 || provider.request.SeasonType != SeasonTypeREG {
		t.Fatalf("request did not use ESPN metadata: %+v", provider.request)
	}
	if len(publisher.texts) != 0 {
		t.Fatal("published previous opponent's report")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale opponent modified state")
	}
}
