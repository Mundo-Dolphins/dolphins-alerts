package espn

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestMonitorFinishesOneGameAndNotifiesNext(t *testing.T) {
	status := StatusInProgress
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(GameDetailResponse{Status: Status{Type: StatusType{Name: status}}, Competitions: []Competition{notificationCompetition("24", "31")}})
	}))
	defer server.Close()
	var notifications []string
	gm := NewGameMonitor(t.TempDir()+"/cache.json", GameCallbacks{
		OnGameStart: func(g GameCache, _, _ string) error { notifications = append(notifications, "start:"+g.ID); return nil },
		OnGameEnd:   func(g GameCache, _, _ string) error { notifications = append(notifications, "end:"+g.ID); return nil },
	}, "", "")
	gm.client.gameDetailURL = server.URL + "/%s"
	gm.client.cache.Games["first"] = GameCache{ID: "first", Date: time.Now().Add(-time.Minute), Status: StatusScheduled}
	gm.pollGames()
	status = StatusFinal
	gm.pollGames()
	if gm.activeGame != nil {
		t.Fatal("finished game still blocks the monitor")
	}
	gm.client.cache.Games["next"] = GameCache{ID: "next", Date: time.Now().Add(-time.Hour), Status: StatusScheduled}
	status = StatusInProgress
	gm.pollGames()
	status = StatusFinal
	gm.pollGames()
	if want := []string{"start:first", "end:first", "start:next", "end:next"}; !reflect.DeepEqual(notifications, want) {
		t.Fatalf("notifications=%v, want %v", notifications, want)
	}
	restored := NewGameMonitor(gm.cacheFile, GameCallbacks{}, "", "")
	if restored.client.cache.Games["next"].Status != StatusFinal {
		t.Fatal("final status was not persisted")
	}
}

func TestDiscoveryPreservesProgressAndRecoversAfterRestart(t *testing.T) {
	now := time.Now().UTC()
	event := Event{ID: "live", Date: now.Add(-time.Minute).Format(time.RFC3339), Status: Status{Type: StatusType{Name: StatusInProgress}}, Competitions: []Competition{notificationCompetition("14", "7")}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/detail/live" {
			_ = json.NewEncoder(w).Encode(GameDetailResponse{Status: event.Status, Competitions: event.Competitions})
			return
		}
		_ = json.NewEncoder(w).Encode(ScoreboardResponse{Events: []Event{event}})
	}))
	defer server.Close()
	starts, scores := 0, 0
	gm := NewGameMonitor(t.TempDir()+"/cache.json", GameCallbacks{
		OnGameStart: func(GameCache, string, string) error { starts++; return nil },
		OnScoring:   func(GameCache, ScoringPlay, string, string, string, string) error { scores++; return nil },
	}, "", "")
	gm.client.scoreboardURL = server.URL
	gm.client.gameDetailURL = server.URL + "/detail/%s"
	gm.client.cache.Games["live"] = GameCache{ID: "live", Date: now, Status: StatusScheduled, LastHomeScore: 14, LastAwayScore: 7, LastPeriod: 1, SeenScoringPlays: map[string]bool{"play": true}}
	if err := gm.loadWeeklyGames(); err != nil {
		t.Fatal(err)
	}
	cached := gm.client.cache.Games["live"]
	if cached.Status != StatusScheduled || cached.LastHomeScore != 14 || !cached.SeenScoringPlays["play"] {
		t.Fatalf("discovery erased notification state: %+v", cached)
	}
	gm.pollGames()
	if starts != 1 || gm.activeGame == nil {
		t.Fatal("discovered transition was lost")
	}
	restarted := NewGameMonitor(gm.cacheFile, gm.callbacks, "", "")
	restarted.client.gameDetailURL = gm.client.gameDetailURL
	restarted.pollGames()
	restarted.pollGames()
	if restarted.activeGame == nil || starts != 1 || scores != 0 {
		t.Fatalf("resume failed or duplicated notifications: starts=%d scores=%d", starts, scores)
	}
}

func TestFinalNotificationFailureRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(GameDetailResponse{Status: Status{Type: StatusType{Name: StatusFinal}}, Competitions: []Competition{notificationCompetition("24", "31")}})
	}))
	defer server.Close()
	attempts := 0
	gm := NewGameMonitor(t.TempDir()+"/cache.json", GameCallbacks{OnGameEnd: func(GameCache, string, string) error {
		attempts++
		if attempts == 1 {
			return errors.New("telegram unavailable")
		}
		return nil
	}}, "", "")
	gm.client.gameDetailURL = server.URL + "/%s"
	gm.startActiveGameMonitoring(GameCache{ID: "live", Status: StatusInProgress})
	if err := gm.monitorActiveGame(); err == nil {
		t.Fatal("expected send failure")
	}
	if err := gm.monitorActiveGame(); err != nil {
		t.Fatal(err)
	}
	if gm.activeGame != nil || attempts != 2 {
		t.Fatal("failed final notification was not retried")
	}
}

func TestDailyScanUsesNextSixAM(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Madrid")
	for _, tc := range []struct {
		now   string
		delay time.Duration
	}{
		{"2026-10-13T05:00:00+02:00", time.Hour},
		{"2026-10-13T15:00:00+02:00", 15 * time.Hour},
		{"2026-10-13T06:00:00+02:00", 24 * time.Hour},
		{"2026-10-24T06:00:00+02:00", 25 * time.Hour},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		if got := untilNextDailyScan(now.In(loc)); got != tc.delay {
			t.Fatalf("%s: %v, want %v", tc.now, got, tc.delay)
		}
	}
}

func TestRecoveryOfOldGameDoesNotSendLateResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(GameDetailResponse{Status: Status{Type: StatusType{Name: StatusFinal}}})
	}))
	defer server.Close()
	finals := 0
	gm := NewGameMonitor(t.TempDir()+"/cache.json", GameCallbacks{OnGameEnd: func(GameCache, string, string) error { finals++; return nil }}, "", "")
	gm.client.gameDetailURL = server.URL + "/%s"
	gm.client.cache.Games["old"] = GameCache{ID: "old", Status: StatusInProgress, Date: time.Now().Add(-48 * time.Hour)}
	gm.pollGames()
	if finals != 0 || gm.client.cache.Games["old"].Status != StatusFinal {
		t.Fatal("recovered game sent an obsolete result or stayed active")
	}
}

func TestScoringFailureDoesNotAdvanceNotifiedScore(t *testing.T) {
	attempts := 0
	gm := &GameMonitor{client: NewClient(), activeGame: &GameCache{ID: "live", LastHomeScore: 7, LastAwayScore: 3}, callbacks: GameCallbacks{OnScoring: func(GameCache, ScoringPlay, string, string, string, string) error {
		attempts++
		if attempts == 1 {
			return errors.New("telegram unavailable")
		}
		return nil
	}}}
	detail := &GameDetailResponse{Status: Status{Period: 1}, Competitions: []Competition{notificationCompetition("14", "3")}}
	if err := gm.checkScoringAndQuarters(detail); err == nil {
		t.Fatal("expected send failure")
	}
	if gm.activeGame.LastHomeScore != 7 {
		t.Fatal("failed notification advanced the score")
	}
	if err := gm.checkScoringAndQuarters(detail); err != nil {
		t.Fatal(err)
	}
	if err := gm.checkScoringAndQuarters(detail); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || gm.activeGame.LastHomeScore != 14 {
		t.Fatal("scoring notification was lost or repeated after success")
	}
}
