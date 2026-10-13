package espn

import (
	"strings"
	"testing"
	"time"
)

func TestGetTeamEmoji_KnownAndUnknown(t *testing.T) {
	if emoji := getTeamEmoji("Miami Dolphins"); !strings.Contains(emoji, "🐬") {
		t.Fatalf("expected dolphin emoji for Miami Dolphins, got %q", emoji)
	}
	if emoji := getTeamEmoji("Nonexistent Team X"); !strings.Contains(emoji, "🏈") {
		t.Fatalf("expected default ball emoji for unknown team, got %q", emoji)
	}
}

func TestFormatGameMessage_IncludesTeamsVenueAndWeather(t *testing.T) {
	away := &Competitor{Team: Team{DisplayName: "Away Team"}, Record: []Record{{Summary: "3-1"}}}
	home := &Competitor{Team: Team{DisplayName: "Home Team"}, Record: []Record{{Summary: "2-2"}}}
	g := GameCache{ID: "g1", Date: time.Now(), Venue: &Venue{FullName: "Stadium", Address: Address{City: "City", State: "ST"}}, Weather: &Weather{DisplayValue: "Sunny", Temperature: 86}, AwayTeam: away, HomeTeam: home}
	msg := FormatGameMessage(g)
	if !strings.Contains(msg, "Away Team") || !strings.Contains(msg, "Home Team") {
		t.Fatalf("FormatGameMessage missing team names: %q", msg)
	}
	if !strings.Contains(msg, "Stadium") || !strings.Contains(msg, "City") {
		t.Fatalf("FormatGameMessage missing venue info: %q", msg)
	}
	if !strings.Contains(msg, "Clima:") {
		t.Fatalf("FormatGameMessage missing weather section: %q", msg)
	}
}

func TestFormatScoringMessage_Basic(t *testing.T) {
	play := ScoringPlay{Type: ScoringType{DisplayName: "Field Goal"}, Team: Team{DisplayName: "Dolphins"}, Text: "Great kick", Clock: Clock{DisplayValue: "12:34"}, Period: Period{DisplayName: "Q2"}}
	msg := FormatScoringMessage(GameCache{}, play, "10", "7")
	if !strings.Contains(msg, "Gol de campo") && !strings.Contains(msg, "Field Goal") {
		t.Fatalf("FormatScoringMessage did not include scoring type translation: %q", msg)
	}
	if !strings.Contains(msg, "Dolphins") || !strings.Contains(msg, "Great kick") {
		t.Fatalf("FormatScoringMessage missing play/team/text: %q", msg)
	}
	if !strings.Contains(msg, "12:34") || !strings.Contains(msg, "Q2") {
		t.Fatalf("FormatScoringMessage missing clock/period: %q", msg)
	}
	if !strings.Contains(msg, "Marcador") {
		t.Fatalf("FormatScoringMessage missing score section: %q", msg)
	}
}
