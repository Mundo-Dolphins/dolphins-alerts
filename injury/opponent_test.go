package injury

import (
	"encoding/json"
	"strings"
	"testing"
)

func withOpponent(report Report, name string, players ...PlayerReport) Report {
	report.OpponentName = name
	report.OpponentPlayers = players
	return report
}

func TestDiffTreatsNewOpponentAsInitial(t *testing.T) {
	previous := testReport("A", PracticeDNP)
	current := withOpponent(testReport("A", PracticeDNP), "Minnesota Vikings",
		testPlayer("V", "Knee", PracticeLP, GameStatusNotListed))

	changes := ReportDiffer{}.Diff(&previous, current)
	if changes.Empty() {
		t.Fatalf("a new opponent section must be published")
	}
	if changes.Initial || len(changes.Added) != 0 {
		t.Fatalf("Miami did not change: %+v", changes)
	}
	if changes.Opponent == nil || !changes.Opponent.Initial || len(changes.Opponent.Added) != 1 {
		t.Fatalf("expected initial opponent changes, got %+v", changes.Opponent)
	}

	if !(ReportDiffer{}.Diff(&current, current).Empty()) {
		t.Fatalf("identical reports with opponent should have no changes")
	}

	updated := withOpponent(testReport("A", PracticeDNP), "Minnesota Vikings",
		testPlayer("V", "Knee", PracticeFP, GameStatusNotListed))
	changes = ReportDiffer{}.Diff(&current, updated)
	if changes.Opponent == nil || changes.Opponent.Initial || len(changes.Opponent.PracticeChanges) != 1 {
		t.Fatalf("expected opponent practice change, got %+v", changes.Opponent)
	}
}

func TestFingerprintIncludesOpponent(t *testing.T) {
	base := testReport("A", PracticeDNP)
	a := withOpponent(base, "Minnesota Vikings", testPlayer("V", "Knee", PracticeLP, GameStatusNotListed))
	b := withOpponent(base, "Minnesota Vikings", testPlayer("V", "Knee", PracticeFP, GameStatusNotListed))
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatalf("opponent changes must produce a new event fingerprint")
	}
	if base.Fingerprint() == a.Fingerprint() {
		t.Fatalf("adding the opponent must produce a new event fingerprint")
	}
}

func TestTelegramRendererCombinedInitial(t *testing.T) {
	report := withOpponent(testReport("Rob Beal Jr.", PracticeDNP), "Minnesota Vikings",
		testPlayer("Viking One", "Heel", PracticeLP, GameStatusNotListed))
	text := TelegramRenderer{}.Render(report, ReportDiffer{}.Diff(nil, report))

	for _, want := range []string{
		"🏥 Primer Injury Report de la semana: Dolphins vs Minnesota Vikings",
		"🐬 MIAMI\n🔴 No entrenaron:\n• Rob Beal Jr.",
		"MINNESOTA VIKINGS\n🟡 Limitados:\n• Viking One — talón",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\n\n\n") {
		t.Fatalf("unexpected double blank line in:\n%s", text)
	}
}

func TestTelegramRendererCombinedUpdateShowsBothTeams(t *testing.T) {
	previous := withOpponent(testReport("Rob Beal Jr.", PracticeDNP), "Minnesota Vikings",
		testPlayer("Viking One", "Knee", PracticeLP, GameStatusNotListed))
	current := withOpponent(testReport("Rob Beal Jr.", PracticeDNP), "Minnesota Vikings",
		testPlayer("Viking One", "Knee", PracticeFP, GameStatusNotListed))
	text := TelegramRenderer{}.Render(current, ReportDiffer{}.Diff(&previous, current))

	for _, want := range []string{
		"🏥 Injury Report actualizado: Dolphins vs Minnesota Vikings",
		"🐬 MIAMI\n🔴 No entrenaron:\n• Rob Beal Jr.",
		"MINNESOTA VIKINGS\n🟢 Participación completa:\n• Viking One — rodilla",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}

	// A snapshot without the opponent (first cycle after enabling it) must
	// also publish both teams.
	legacy := testReport("Rob Beal Jr.", PracticeDNP)
	text = TelegramRenderer{}.Render(current, ReportDiffer{}.Diff(&legacy, current))
	if !strings.Contains(text, "🐬 MIAMI") || !strings.Contains(text, "MINNESOTA VIKINGS") {
		t.Fatalf("expected both teams in:\n%s", text)
	}
}

func TestTelegramRendererCombinedFinal(t *testing.T) {
	miami := testReport("A", PracticeFP)
	miami.Players[0].GameStatus = GameStatusOut
	report := withOpponent(miami, "Minnesota Vikings",
		testPlayer("Viking One", "Knee", PracticeLP, GameStatusQuestionable))
	text := TelegramRenderer{}.Render(report, ReportDiffer{}.Diff(nil, report))

	for _, want := range []string{
		"🏥 Injury Report final: Dolphins vs Minnesota Vikings",
		"🐬 MIAMI\n❌ OUT\n• A",
		"MINNESOTA VIKINGS\n❓ QUESTIONABLE\n• Viking One",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestReportJSONKeepsOpponentAndLegacyCompatibility(t *testing.T) {
	report := withOpponent(testReport("A", PracticeDNP), "Minnesota Vikings",
		testPlayer("V", "Knee", PracticeLP, GameStatusNotListed))
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.OpponentName != "Minnesota Vikings" || len(back.OpponentPlayers) != 1 {
		t.Fatalf("opponent not persisted: %+v", back)
	}

	legacy, _ := json.Marshal(testReport("A", PracticeDNP))
	if strings.Contains(string(legacy), "opponentPlayers") || strings.Contains(string(legacy), "opponentName") {
		t.Fatalf("reports without opponent must keep the previous schema: %s", legacy)
	}
}
