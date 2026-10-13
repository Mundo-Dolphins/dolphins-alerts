package injury

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type TeamCode string

const TeamMIA TeamCode = "MIA"

type SeasonType string

const (
	SeasonTypeREG  SeasonType = "REG"
	SeasonTypePOST SeasonType = "POST"
)

type PracticeStatus string

const (
	PracticeDNP       PracticeStatus = "DNP"
	PracticeLP        PracticeStatus = "LP"
	PracticeFP        PracticeStatus = "FP"
	PracticeNotListed PracticeStatus = "NotListed"
	PracticeUnknown   PracticeStatus = "Unknown"
)

type GameStatus string

const (
	GameStatusOut          GameStatus = "Out"
	GameStatusDoubtful     GameStatus = "Doubtful"
	GameStatusQuestionable GameStatus = "Questionable"
	GameStatusNotListed    GameStatus = "NotListed"
	GameStatusUnknown      GameStatus = "Unknown"
)

type ReportRequest struct {
	Team         TeamCode
	Season       int
	SeasonType   SeasonType
	Week         int
	ForceRefresh bool
}

type PracticeEntry struct {
	Day    string         `json:"day"`
	Status PracticeStatus `json:"status"`
}

type PlayerReport struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Position   string          `json:"position"`
	Injury     string          `json:"injury"`
	Practices  []PracticeEntry `json:"practices"`
	GameStatus GameStatus      `json:"gameStatus"`
}

type Report struct {
	Season      int            `json:"season"`
	SeasonType  SeasonType     `json:"seasonType"`
	Week        int            `json:"week"`
	Team        TeamCode       `json:"team"`
	Opponent    TeamCode       `json:"opponent,omitempty"`
	Source      string         `json:"source"`
	SourceURL   string         `json:"sourceUrl"`
	RetrievedAt time.Time      `json:"retrievedAt"`
	Players     []PlayerReport `json:"players"`
	// OpponentName and OpponentPlayers hold the next opponent's section of the
	// same injury report page, published together with Miami's.
	OpponentName    string         `json:"opponentName,omitempty"`
	OpponentPlayers []PlayerReport `json:"opponentPlayers,omitempty"`
}

// OpponentReport returns the opponent section as a standalone report.
func (r Report) OpponentReport() Report {
	opp := r
	opp.Players = r.OpponentPlayers
	opp.OpponentName = ""
	opp.OpponentPlayers = nil
	return opp
}

func (r Report) SnapshotKey() string {
	return fmt.Sprintf("%d/%s/week-%02d/%s", r.Season, r.SeasonType, r.Week, r.Team)
}

func (r Report) Fingerprint() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%s|%d|%s|%s|", r.Season, r.SeasonType, r.Week, r.Team, r.Opponent)
	writePlayersFingerprint(&b, r.Players)
	if r.OpponentName != "" {
		fmt.Fprintf(&b, "opponent|%s|", r.OpponentName)
		writePlayersFingerprint(&b, r.OpponentPlayers)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func writePlayersFingerprint(b *strings.Builder, in []PlayerReport) {
	players := append([]PlayerReport(nil), in...)
	sort.Slice(players, func(i, j int) bool { return players[i].stableID() < players[j].stableID() })
	for _, p := range players {
		fmt.Fprintf(b, "%s|%s|%s|%s|", p.stableID(), p.Position, strings.ToLower(p.Injury), p.GameStatus)
		for _, pr := range p.Practices {
			fmt.Fprintf(b, "%s=%s,", strings.ToLower(pr.Day), pr.Status)
		}
		b.WriteByte(';')
	}
}

func (p PlayerReport) LatestPractice() PracticeStatus {
	if len(p.Practices) == 0 {
		return PracticeNotListed
	}
	return p.Practices[len(p.Practices)-1].Status
}

func (p PlayerReport) stableID() string {
	if strings.TrimSpace(p.ID) != "" {
		return strings.ToLower(strings.TrimSpace(p.ID))
	}
	return strings.ToLower(strings.TrimSpace(p.Name))
}

func ValidateReport(report Report) error {
	if report.Team == "" {
		return fmt.Errorf("missing team")
	}
	if report.Team != TeamMIA {
		return fmt.Errorf("unexpected team: %s", report.Team)
	}
	if report.Season < 2020 || report.Season > time.Now().Year()+1 {
		return fmt.Errorf("invalid season: %d", report.Season)
	}
	if report.Week < 1 || report.Week > 23 {
		return fmt.Errorf("invalid week: %d", report.Week)
	}
	if report.Source == "" || report.SourceURL == "" {
		return fmt.Errorf("missing source")
	}
	return ValidatePlayers(report.Players)
}

// ValidatePlayers checks the players of one team section.
func ValidatePlayers(players []PlayerReport) error {
	seen := map[string]bool{}
	for _, p := range players {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("player missing name")
		}
		if strings.TrimSpace(p.Position) == "" {
			return fmt.Errorf("player %q missing position", p.Name)
		}
		if p.LatestPractice() == PracticeUnknown {
			return fmt.Errorf("player %q has unknown practice status", p.Name)
		}
		if p.GameStatus == GameStatusUnknown {
			return fmt.Errorf("player %q has unknown game status", p.Name)
		}
		id := p.stableID()
		if seen[id] {
			return fmt.Errorf("duplicate player %q", p.Name)
		}
		seen[id] = true
	}
	return nil
}

// Legacy compatibility types kept for existing tools/tests during the V2 migration.
type InjuryPlayer struct {
	Name             string `json:"name"`
	Position         string `json:"position"`
	Injury           string `json:"injury"`
	InjuryTranslated string `json:"injuryTranslated,omitempty"`
	Team             string `json:"team"`
	Status           string `json:"status"`
	GameStatus       string `json:"gameStatus"`
}

type InjuryReport struct {
	Date    time.Time      `json:"date"`
	Players []InjuryPlayer `json:"players"`
	Hash    string         `json:"hash"`
}

type InjuryCache struct {
	LastReport  *InjuryReport        `json:"lastReport"`
	LastChecked time.Time            `json:"lastChecked"`
	LastHash    string               `json:"lastHash"`
	Notified    map[string]time.Time `json:"notified,omitempty"`
}

func ComputePlayersHash(players []InjuryPlayer) string {
	if len(players) == 0 {
		return ""
	}
	report := Report{Season: time.Now().Year(), SeasonType: SeasonTypeREG, Week: 1, Team: TeamMIA}
	for _, p := range players {
		report.Players = append(report.Players, PlayerReport{
			ID:         p.Name,
			Name:       p.Name,
			Position:   p.Position,
			Injury:     p.Injury,
			Practices:  []PracticeEntry{{Day: "latest", Status: NormalizePracticeStatus(p.Status)}},
			GameStatus: NormalizeGameStatus(p.GameStatus),
		})
	}
	return report.Fingerprint()
}
