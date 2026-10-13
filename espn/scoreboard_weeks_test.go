package espn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGetScoreboardFetchesCurrentAndUpcomingWeeks(t *testing.T) {
	var queries []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if strings.Contains(r.URL.Query().Get("dates"), "-") {
			// ESPN rejects date ranges with 400 "Failed to get events endpoint."
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := ScoreboardResponse{}
		q := r.URL.Query()
		switch {
		case r.URL.RawQuery == "":
			resp = ScoreboardResponse{
				Season: ScoreboardSeason{Type: 2, Year: 2026},
				Week:   ScoreboardWeek{Number: 17},
				Events: []Event{{ID: "week17"}},
			}
		case q.Get("seasontype") == "2" && q.Get("week") == "18":
			resp.Events = []Event{{ID: "week17"}, {ID: "week18"}}
		case q.Get("seasontype") == "3" && q.Get("week") == "1":
			resp.Events = []Event{{ID: "wildcard"}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := NewClient()
	c.scoreboardURL = ts.URL

	sb, err := c.GetScoreboard()
	if err != nil {
		t.Fatalf("GetScoreboard error: %v", err)
	}
	var ids []string
	for _, ev := range sb.Events {
		ids = append(ids, ev.ID)
	}
	if !reflect.DeepEqual(ids, []string{"week17", "week18", "wildcard"}) {
		t.Fatalf("expected deduplicated events across weeks, got %v", ids)
	}
	// From week 17 only six weeks remain until the Super Bowl.
	want := []string{"", "seasontype=2&week=18", "seasontype=3&week=1", "seasontype=3&week=2",
		"seasontype=3&week=3", "seasontype=3&week=4", "seasontype=3&week=5"}
	if !reflect.DeepEqual(queries, want) {
		t.Fatalf("unexpected week queries: %v", queries)
	}
}

func TestNextScoreboardWeek(t *testing.T) {
	cases := []struct{ seasonType, week, wantType, wantWeek int }{
		{1, 2, 1, 3},
		{1, 4, 2, 1},
		{2, 4, 2, 5},
		{2, 18, 3, 1},
		{3, 4, 3, 5},
	}
	for _, c := range cases {
		gotType, gotWeek, ok := nextScoreboardWeek(c.seasonType, c.week)
		if !ok || gotType != c.wantType || gotWeek != c.wantWeek {
			t.Errorf("next(%d,%d) = %d,%d,%v; want %d,%d", c.seasonType, c.week, gotType, gotWeek, ok, c.wantType, c.wantWeek)
		}
	}
	if _, _, ok := nextScoreboardWeek(3, 5); ok {
		t.Errorf("no week after the Super Bowl")
	}
}
