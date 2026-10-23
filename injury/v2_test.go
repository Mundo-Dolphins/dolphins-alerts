package injury

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReportDifferSemanticChanges(t *testing.T) {
	prev := &Report{Players: []PlayerReport{
		testPlayer("A", "Hamstring", PracticeDNP, GameStatusQuestionable),
		testPlayer("B", "Knee", PracticeLP, GameStatusNotListed),
		testPlayer("C", "Ankle", PracticeFP, GameStatusNotListed),
		testPlayer("D", "Groin", PracticeLP, GameStatusQuestionable),
	}}
	cur := Report{Players: []PlayerReport{
		testPlayer("A", "Hamstring", PracticeLP, GameStatusOut),
		testPlayer("B", "Knee", PracticeFP, GameStatusNotListed),
		testPlayer("C", "Shoulder", PracticeLP, GameStatusNotListed),
		testPlayer("E", "Foot", PracticeDNP, GameStatusNotListed),
	}}
	changes := ReportDiffer{}.Diff(prev, cur)
	if len(changes.Added) != 1 || changes.Added[0].Name != "E" {
		t.Fatalf("added mismatch: %+v", changes.Added)
	}
	if len(changes.Removed) != 1 || changes.Removed[0].Name != "D" {
		t.Fatalf("removed mismatch: %+v", changes.Removed)
	}
	if len(changes.PracticeChanges) != 3 {
		t.Fatalf("practice changes mismatch: %+v", changes.PracticeChanges)
	}
	if changes.PracticeChanges[0].Trend != DirectionImproved || changes.PracticeChanges[2].Trend != DirectionWorsened {
		t.Fatalf("unexpected trends: %+v", changes.PracticeChanges)
	}
	if len(changes.InjuryChanges) != 1 || len(changes.GameStatusChanges) != 1 {
		t.Fatalf("expected injury and game status changes: %+v", changes)
	}
	if !(ReportDiffer{}.Diff(&cur, cur).Empty()) {
		t.Fatalf("identical reports should have no changes")
	}
}

func TestMonitorIdempotency(t *testing.T) {
	store := NewJSONReportStore(t.TempDir() + "/injury.json")
	pub := &capturePublisher{}
	monitor := NewMonitor(t.TempDir()+"/unused.json", "", "")
	monitor.AllowNotifyOutsidePublishDays = true
	monitor.SetStore(store)
	monitor.SetPublisher(pub)
	monitor.SetProvider(&sequenceProvider{reports: []Report{
		testReport("A", PracticeDNP),
		testReport("A", PracticeDNP),
		testReport("A", PracticeDNP),
		testReport("A", PracticeLP),
		testReport("A", PracticeLP),
	}})
	for i := 0; i < 5; i++ {
		_, err := monitor.Check(context.Background(), false, false)
		if err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
	}
	if len(pub.texts) != 2 {
		t.Fatalf("expected initial and one update, got %d: %+v", len(pub.texts), pub.texts)
	}
}

func TestTelegramRendererInitialAndFinal(t *testing.T) {
	report := testReport("A", PracticeDNP)
	report.Players = append(report.Players, testPlayer(strings.Repeat("LongName", 30), "Knee", PracticeLP, GameStatusQuestionable))
	initial := ReportDiffer{}.Diff(nil, report)
	tg := TelegramRenderer{}.Render(report, initial)
	if !strings.Contains(tg, "No entrenaron") || strings.Contains(tg, "Participación completa") {
		t.Fatalf("unexpected telegram initial: %s", tg)
	}
	finalReport := testReport("A", PracticeFP)
	finalReport.Players[0].GameStatus = GameStatusOut
	final := ReportChanges{Final: true}
	if !strings.Contains(TelegramRenderer{}.Render(finalReport, final), "OUT") {
		t.Fatalf("expected final OUT rendering")
	}
}

type capturePublisher struct{ texts []string }

func (p *capturePublisher) Publish(_ context.Context, text string) error {
	p.texts = append(p.texts, text)
	return nil
}

type sequenceProvider struct {
	reports []Report
	index   int
}

func (p *sequenceProvider) Fetch(_ context.Context, _ ReportRequest) (Report, error) {
	if p.index >= len(p.reports) {
		return p.reports[len(p.reports)-1], nil
	}
	report := p.reports[p.index]
	p.index++
	return report, nil
}

func testReport(name string, status PracticeStatus) Report {
	return Report{
		Season:      2026,
		SeasonType:  SeasonTypeREG,
		Week:        1,
		Team:        TeamMIA,
		Source:      "nfl",
		SourceURL:   "https://www.nfl.com/injuries/",
		RetrievedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		Players:     []PlayerReport{testPlayer(name, "Hamstring", status, GameStatusNotListed)},
	}
}

func testPlayer(name string, injury string, practice PracticeStatus, game GameStatus) PlayerReport {
	return PlayerReport{
		ID:         strings.ToLower(name),
		Name:       name,
		Position:   "WR",
		Injury:     injury,
		Practices:  []PracticeEntry{{Day: "latest", Status: practice}},
		GameStatus: game,
	}
}

func TestMonitorWaitsForReportToStabilizeBeforePublishing(t *testing.T) {
	pub := &capturePublisher{}
	monitor := NewMonitor(t.TempDir()+"/unused.json", "", "")
	monitor.AllowNotifyOutsidePublishDays = true
	monitor.SetStore(NewJSONReportStore(t.TempDir() + "/injury.json"))
	monitor.SetPublisher(pub)
	// The page is updated in several steps: DNP -> LP -> FP, and only the
	// last one stays unchanged across two consecutive checks.
	monitor.SetProvider(&sequenceProvider{reports: []Report{
		testReport("A", PracticeDNP),
		testReport("A", PracticeLP),
		testReport("A", PracticeFP),
		testReport("A", PracticeFP),
		testReport("A", PracticeFP),
	}})
	for i := 0; i < 5; i++ {
		if _, err := monitor.Check(context.Background(), false, false); err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
		wantPublished := 0
		if i >= 3 {
			wantPublished = 1
		}
		if len(pub.texts) != wantPublished {
			t.Fatalf("after check %d expected %d publications, got %d", i, wantPublished, len(pub.texts))
		}
	}
}

func TestMonitorForceDoesNotWaitForStability(t *testing.T) {
	pub := &capturePublisher{}
	monitor := NewMonitor(t.TempDir()+"/unused.json", "", "")
	monitor.AllowNotifyOutsidePublishDays = true
	monitor.SetStore(NewJSONReportStore(t.TempDir() + "/injury.json"))
	monitor.SetPublisher(pub)
	monitor.SetProvider(&sequenceProvider{reports: []Report{testReport("A", PracticeDNP)}})
	if _, err := monitor.Check(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if len(pub.texts) != 1 {
		t.Fatalf("forced check should publish immediately, got %d", len(pub.texts))
	}
}
