package injury

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const nflverseInjuriesCSV = `season,season_type,game_type,team,week,gsis_id,position,full_name,first_name,last_name,report_primary_injury,report_secondary_injury,report_status,practice_primary_injury,practice_secondary_injury,practice_status
2026,REG,REG,MIA,3,00-1,DE,Rob Beal Jr.,Rob,Beal,Hamstring,,Out,Hamstring,,Did Not Participate In Practice
2026,REG,REG,MIA,4,00-1,DE,Rob Beal Jr.,Rob,Beal,,,,Hamstring,,Did Not Participate In Practice
2026,REG,REG,MIA,4,00-2,RB,Jaylen Wright,Jaylen,Wright,,,,Stinger,Foot,Limited Participation in Practice
2026,REG,REG,MIA,4,00-3,CB,Julius Brents,Julius,Brents,Knee,Heel,Questionable,,,Full Participation in Practice
2026,REG,REG,MIN,4,00-4,WR,Justin Jefferson,Justin,Jefferson,,,,Ankle,,Did Not Participate In Practice
2026,REG,REG,MIN,4,00-5,RB,Aaron Jones,Aaron,Jones,,,,Not injury related - resting player,,Limited Participation in Practice
2026,REG,REG,BUF,4,00-6,QB,Other Player,Other,Player,,,,Knee,,Limited Participation in Practice
`

const nflverseGamesCSV = `game_id,season,game_type,week,gameday,weekday,gametime,away_team,away_score,home_team,home_score
2026_03_KC_MIA,2026,REG,3,2026-09-27,Sunday,13:00,KC,24,MIA,10
2026_04_MIA_MIN,2026,REG,4,2026-10-04,Sunday,16:05,MIA,,MIN,
2026_04_BUF_NE,2026,REG,4,2026-10-04,Sunday,13:00,BUF,,NE,
`

func newNFLVerseServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	gamesRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/injuries_2026.csv":
			w.Header().Set("Last-Modified", "Thu, 01 Oct 2026 06:01:41 GMT")
			_, _ = w.Write([]byte(nflverseInjuriesCSV))
		case "/games.csv":
			gamesRequests++
			_, _ = w.Write([]byte(nflverseGamesCSV))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &gamesRequests
}

func TestNFLVerseProviderBuildsReportWithOpponent(t *testing.T) {
	srv, gamesRequests := newNFLVerseServer(t)
	provider := NewNFLVerseProvider(srv.Client())
	provider.injuriesURL = srv.URL + "/injuries_%d.csv"
	provider.gamesURL = srv.URL + "/games.csv"

	req := ReportRequest{Team: TeamMIA, Season: 2026, SeasonType: SeasonTypeREG, Week: 4}
	report, err := provider.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if report.Source != SourceNFLVerse || report.SourceURL != srv.URL+"/injuries_2026.csv" {
		t.Fatalf("unexpected source: %s %s", report.Source, report.SourceURL)
	}
	if want := time.Date(2026, 10, 1, 6, 1, 41, 0, time.UTC); !report.RetrievedAt.Equal(want) {
		t.Fatalf("RetrievedAt must be the data's Last-Modified time, got %s", report.RetrievedAt)
	}
	if len(report.Players) != 3 {
		t.Fatalf("expected 3 Miami players for week 4, got %+v", report.Players)
	}
	byName := map[string]PlayerReport{}
	for _, p := range report.Players {
		byName[p.Name] = p
	}
	if p := byName["Jaylen Wright"]; p.Injury != "Stinger / Foot" || p.LatestPractice() != PracticeLP || p.Position != "RB" {
		t.Fatalf("unexpected Wright: %+v", p)
	}
	if p := byName["Julius Brents"]; p.Injury != "Knee / Heel" || p.GameStatus != GameStatusQuestionable || p.LatestPractice() != PracticeFP {
		t.Fatalf("unexpected Brents: %+v", p)
	}
	if byName["Rob Beal Jr."].ID != player("Rob Beal Jr.", "DE", "", PracticeDNP, GameStatusNotListed).ID {
		t.Fatalf("player IDs must match the HTML provider so diffs stay stable")
	}
	if report.OpponentName != "Minnesota Vikings" || len(report.OpponentPlayers) != 2 {
		t.Fatalf("unexpected opponent: %q %+v", report.OpponentName, report.OpponentPlayers)
	}

	if _, err := provider.Fetch(context.Background(), req); err != nil {
		t.Fatalf("second Fetch returned error: %v", err)
	}
	if *gamesRequests != 1 {
		t.Fatalf("the schedule must be cached per week, got %d downloads", *gamesRequests)
	}
}

type stubProvider struct {
	report Report
	err    error
	calls  int
}

func (s *stubProvider) Fetch(context.Context, ReportRequest) (Report, error) {
	s.calls++
	return s.report, s.err
}

func TestFallbackProvider(t *testing.T) {
	req := ReportRequest{Team: TeamMIA, Season: 2026, SeasonType: SeasonTypeREG, Week: 4}

	primary := &stubProvider{report: Report{Source: "nfl"}}
	secondary := &stubProvider{report: Report{Source: SourceNFLVerse}}
	report, err := NewFallbackProvider(primary, secondary).Fetch(context.Background(), req)
	if err != nil || report.Source != "nfl" || secondary.calls != 0 {
		t.Fatalf("primary success must not use the fallback: %+v %v calls=%d", report, err, secondary.calls)
	}

	primary.err = errors.New("scraper down")
	report, err = NewFallbackProvider(primary, secondary).Fetch(context.Background(), req)
	if err != nil || report.Source != SourceNFLVerse {
		t.Fatalf("expected fallback report, got %+v %v", report, err)
	}

	secondary.err = errors.New("github down")
	_, err = NewFallbackProvider(primary, secondary).Fetch(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "scraper down") || !strings.Contains(err.Error(), "github down") {
		t.Fatalf("expected both errors, got %v", err)
	}
}

func TestDiffIgnoresInjuryWordingAcrossSources(t *testing.T) {
	previous := testReport("A", PracticeDNP)
	previous.Players[0].Injury = "Right Hamstring"
	current := testReport("A", PracticeDNP)
	current.Source = SourceNFLVerse
	current.Players[0].Injury = "Hamstring"

	if changes := (ReportDiffer{}).Diff(&previous, current); !changes.Empty() {
		t.Fatalf("a source switch must not publish injury wording changes: %+v", changes)
	}

	current.Players[0].Practices = []PracticeEntry{{Day: "latest", Status: PracticeLP}}
	if changes := (ReportDiffer{}).Diff(&previous, current); len(changes.PracticeChanges) != 1 {
		t.Fatalf("practice changes must still be detected: %+v", changes)
	}
}

func TestTelegramRendererWarnsOnFallbackSource(t *testing.T) {
	report := testReport("A", PracticeDNP)
	report.Source = SourceNFLVerse
	report.SourceURL = "https://github.com/nflverse/nflverse-data/releases/download/injuries/injuries_2026.csv"
	text := TelegramRenderer{}.Render(report, ReportDiffer{}.Diff(nil, report))
	if !strings.Contains(text, "⚠️ Fuente alternativa (nflverse)") {
		t.Fatalf("expected fallback warning in:\n%s", text)
	}

	report.Source = "nfl"
	if strings.Contains(TelegramRenderer{}.Render(report, ReportDiffer{}.Diff(nil, report)), "Fuente alternativa") {
		t.Fatalf("the primary source must not show the warning")
	}
}

func TestMonitorSkipsFallbackDataOlderThanSnapshot(t *testing.T) {
	published := time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC)
	primary := testReport("A", PracticeLP)
	primary.RetrievedAt = published

	stale := testReport("A", PracticeDNP)
	stale.Source = SourceNFLVerse
	stale.RetrievedAt = published.Add(-16 * time.Hour)

	fresh := testReport("A", PracticeFP)
	fresh.Source = SourceNFLVerse
	fresh.RetrievedAt = published.Add(8 * time.Hour)

	pub := &capturePublisher{}
	monitor := NewMonitor(t.TempDir()+"/unused.json", "", "")
	monitor.AllowNotifyOutsidePublishDays = true
	monitor.SetStore(NewJSONReportStore(t.TempDir() + "/injury.json"))
	monitor.SetPublisher(pub)
	monitor.SetProvider(&sequenceProvider{reports: []Report{primary, stale, fresh}})

	for i := 0; i < 3; i++ {
		if _, err := monitor.Check(context.Background(), false, false); err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
	}
	if len(pub.texts) != 2 {
		t.Fatalf("expected the primary report and the newer fallback data only, got %d: %+v", len(pub.texts), pub.texts)
	}
	if strings.Contains(pub.texts[1], "no entreno") || !strings.Contains(pub.texts[1], "limitado → participacion completa") {
		t.Fatalf("stale fallback data must not be published, got:\n%s", pub.texts[1])
	}
}
