package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func notificationCompetition(homeScore, awayScore string) Competition {
	return Competition{Competitors: []Competitor{
		{HomeAway: "home", Score: homeScore, Team: Team{ID: "15", DisplayName: "Miami Dolphins"}},
		{HomeAway: "away", Score: awayScore, Team: Team{ID: "2", DisplayName: "Buffalo Bills"}},
	}}
}

func assertSpoiler(t *testing.T, message string) (string, string) {
	t.Helper()
	visible, rest, ok := strings.Cut(message, "<tg-spoiler>")
	if !ok {
		t.Fatalf("missing spoiler: %s", message)
	}
	hidden, after, ok := strings.Cut(rest, "</tg-spoiler>")
	if !ok || after != "" {
		t.Fatalf("invalid spoiler: %s", message)
	}
	return visible, hidden
}

func TestNotificationsHaveTeamsAndHideResults(t *testing.T) {
	game := GameCache{}
	updateGameCompetition(&game, notificationCompetition("6", "0"))
	old := sendGameMessage
	t.Cleanup(func() { sendGameMessage = old })
	var messages []string
	sendGameMessage = func(_, _, message string) error { messages = append(messages, message); return nil }
	if err := handleGameStart(game, "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[0], "El partido entre Buffalo Bills y Miami Dolphins ha comenzado.") || strings.Count(messages[0], "comenzado") != 1 {
		t.Fatalf("invalid start: %s", messages[0])
	}
	play := ScoringPlay{Team: game.HomeTeam.Team, Type: ScoringType{DisplayName: "Anotación"}}
	if err := handleScoring(game, play, "6", "0", "", ""); err != nil {
		t.Fatal(err)
	}
	visible, hidden := assertSpoiler(t, messages[1])
	if visible != "🎯 ¡ANOTACIÓN!\n\n🆚 Buffalo Bills vs Miami Dolphins\n\n" {
		t.Fatalf("unexpected visible scoring text: %s", visible)
	}
	if !strings.Contains(hidden, "Anota Miami Dolphins") || !strings.Contains(hidden, "Buffalo Bills 0 - Miami Dolphins 6") || strings.Count(hidden, "Marcador:") != 1 {
		t.Fatalf("invalid scoring details: %s", hidden)
	}
	if err := handleGameEnd(game, "", ""); err != nil {
		t.Fatal(err)
	}
	visible, hidden = assertSpoiler(t, messages[2])
	if strings.Contains(visible, "comenzado") || !strings.Contains(visible, "ha terminado") || !strings.Contains(hidden, "Buffalo Bills 0 - Miami Dolphins 6") {
		t.Fatalf("invalid end: %s", messages[2])
	}
	for _, status := range []string{StatusInProgress, StatusFinal} {
		if err := handleStatusChange(game, StatusScheduled, status, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(messages) != 3 {
		t.Fatalf("start/end duplicated: %v", messages)
	}
	if err := handleStatusChange(game, StatusInProgress, StatusDelayed, "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[3], "Retrasado") || strings.Contains(messages[3], "STATUS_") || strings.Contains(messages[3], "comenzado") {
		t.Fatalf("invalid status: %s", messages[3])
	}
}

func TestNotificationHTMLCannotExposeHiddenDetails(t *testing.T) {
	game := GameCache{AwayTeam: &Competitor{Team: Team{DisplayName: "Away & Co"}}, HomeTeam: &Competitor{Team: Team{DisplayName: "Home <team>"}}}
	play := ScoringPlay{Team: Team{DisplayName: "Home <team>"}, Text: "</tg-spoiler><b>6-0</b>"}
	message := FormatScoringMessage(game, play, "6", "0")
	visible, hidden := assertSpoiler(t, message)
	if !strings.Contains(visible, "Away &amp; Co vs Home &lt;team&gt;") || !strings.Contains(hidden, "&lt;/tg-spoiler&gt;&lt;b&gt;6-0&lt;/b&gt;") {
		t.Fatalf("unescaped HTML: %s", message)
	}
}

func TestEventCacheRetainsMatchup(t *testing.T) {
	gm := &GameMonitor{}
	game := gm.eventToGameCache(Event{ID: "g1", Date: "2026-10-11T17:00:00Z", Competitions: []Competition{notificationCompetition("0", "0")}})
	if game.HomeTeam == nil || game.AwayTeam == nil || game.HomeTeam.Team.DisplayName != "Miami Dolphins" || game.AwayTeam.Team.DisplayName != "Buffalo Bills" {
		t.Fatalf("missing teams: %+v", game)
	}
}

func TestScoreChangeAttributionAndCorrections(t *testing.T) {
	for _, tc := range []struct {
		name, home, away, team, kind string
		previousHome, previousAway   int
		calls                        int
	}{
		{name: "home scores", home: "6", away: "0", team: "Miami Dolphins", kind: "scoring", calls: 1},
		{name: "away scores", home: "0", away: "3", team: "Buffalo Bills", kind: "scoring", calls: 1},
		{name: "both score", home: "7", away: "3", kind: "scoring", calls: 1},
		{name: "score correction", home: "6", away: "0", previousHome: 7, kind: "score_correction", calls: 1},
		{name: "unchanged", home: "7", away: "3", previousHome: 7, previousAway: 3},
		{name: "missing score", home: "", away: "3", previousHome: 7, previousAway: 3},
		{name: "invalid score", home: "unknown", away: "3", previousHome: 7, previousAway: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gm := &GameMonitor{client: NewClient(), activeGame: &GameCache{ID: "g1", LastHomeScore: tc.previousHome, LastAwayScore: tc.previousAway}}
			calls := 0
			gm.callbacks.OnScoring = func(game GameCache, play ScoringPlay, home, away, _, _ string) error {
				calls++
				if home != tc.home || away != tc.away || play.Team.DisplayName != tc.team || play.Type.Name != tc.kind {
					t.Fatalf("invalid play %+v, scores %s/%s", play, home, away)
				}
				visible, hidden := assertSpoiler(t, FormatScoringMessage(game, play, home, away))
				if strings.Contains(visible, "Marcador:") || strings.Contains(hidden, "Score updated") {
					t.Fatal("score leaked or English placeholder remains")
				}
				if tc.name == "both score" && !strings.Contains(hidden, "Ambos equipos") {
					t.Fatal("both scoring teams must be acknowledged")
				}
				return nil
			}
			detail := &GameDetailResponse{Competitions: []Competition{notificationCompetition(tc.home, tc.away)}, Status: Status{Period: 1, DisplayClock: "10:00"}}
			for i := 0; i < 2; i++ {
				if err := gm.checkScoringAndQuarters(detail); err != nil {
					t.Fatal(err)
				}
			}
			if calls != tc.calls {
				t.Fatalf("callbacks=%d want=%d", calls, tc.calls)
			}
		})
	}
}

func TestFinalNotificationUsesLatestESPNScore(t *testing.T) {
	detail := GameDetailResponse{Status: Status{Type: StatusType{Name: StatusFinal}}, Competitions: []Competition{notificationCompetition("21", "17")}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(detail) }))
	t.Cleanup(server.Close)
	gm := &GameMonitor{client: NewClient(), cacheFile: t.TempDir() + "/cache.json", activeGame: &GameCache{ID: "g1", Status: StatusInProgress, LastHomeScore: 14, LastAwayScore: 17}}
	gm.client.gameDetailURL = server.URL + "/%s"
	var final string
	gm.callbacks.OnGameEnd = func(game GameCache, _, _ string) error { final = FormatGameEndMessage(game); return nil }
	if err := gm.monitorActiveGame(); err != nil {
		t.Fatal(err)
	}
	_, hidden := assertSpoiler(t, final)
	if !strings.Contains(hidden, "Buffalo Bills 17 - Miami Dolphins 21") || gm.activeGame != nil {
		t.Fatalf("invalid final: %s", final)
	}
}
