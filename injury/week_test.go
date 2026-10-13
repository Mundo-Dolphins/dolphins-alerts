package injury

import (
	"testing"
	"time"
)

func TestInferWeekUsesNFLRegularSeasonBoundary(t *testing.T) {
	tests := []struct {
		name string
		date string
		want int
	}{
		{name: "before 2026 kickoff", date: "2026-09-07T12:00:00-04:00", want: 1},
		{name: "2026 week one starts", date: "2026-09-08T12:00:00-04:00", want: 1},
		{name: "2026 week one ends", date: "2026-09-14T12:00:00-04:00", want: 1},
		{name: "2026 week two starts", date: "2026-09-15T12:00:00-04:00", want: 2},
		{name: "reported regression", date: "2026-09-17T10:00:00-04:00", want: 2},
		{name: "2026 week three starts", date: "2026-09-22T12:00:00-04:00", want: 3},
		{name: "2025 week two", date: "2025-09-10T12:00:00-04:00", want: 2},
		{name: "season continues in January", date: "2027-01-05T12:00:00-05:00", want: 18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tt.date)
			if err != nil {
				t.Fatal(err)
			}
			if got := inferWeek(now); got != tt.want {
				t.Fatalf("inferWeek(%s) = %d, want %d", tt.date, got, tt.want)
			}
		})
	}
}

func TestInferSeasonKeepsJanuaryInPreviousNFLSeason(t *testing.T) {
	now := time.Date(2027, time.January, 5, 12, 0, 0, 0, time.UTC)
	if got := inferSeason(now); got != 2026 {
		t.Fatalf("inferSeason(%s) = %d, want 2026", now, got)
	}
}
