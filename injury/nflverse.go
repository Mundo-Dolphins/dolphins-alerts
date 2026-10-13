package injury

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SourceNFLVerse marks reports built from the nflverse fallback.
const SourceNFLVerse = "nflverse"

const (
	nflverseInjuriesURL = "https://github.com/nflverse/nflverse-data/releases/download/injuries/injuries_%d.csv"
	nflverseGamesURL    = "https://github.com/nflverse/nfldata/raw/master/data/games.csv"
)

// nflverseTeamNames maps nflverse team abbreviations to the club names used
// on the official injury report.
var nflverseTeamNames = map[string]string{
	"ARI": "Arizona Cardinals", "ATL": "Atlanta Falcons", "BAL": "Baltimore Ravens", "BUF": "Buffalo Bills",
	"CAR": "Carolina Panthers", "CHI": "Chicago Bears", "CIN": "Cincinnati Bengals", "CLE": "Cleveland Browns",
	"DAL": "Dallas Cowboys", "DEN": "Denver Broncos", "DET": "Detroit Lions", "GB": "Green Bay Packers",
	"HOU": "Houston Texans", "IND": "Indianapolis Colts", "JAX": "Jacksonville Jaguars", "KC": "Kansas City Chiefs",
	"LA": "Los Angeles Rams", "LAC": "Los Angeles Chargers", "LV": "Las Vegas Raiders", "MIA": "Miami Dolphins",
	"MIN": "Minnesota Vikings", "NE": "New England Patriots", "NO": "New Orleans Saints", "NYG": "New York Giants",
	"NYJ": "New York Jets", "PHI": "Philadelphia Eagles", "PIT": "Pittsburgh Steelers", "SEA": "Seattle Seahawks",
	"SF": "San Francisco 49ers", "TB": "Tampa Bay Buccaneers", "TEN": "Tennessee Titans", "WAS": "Washington Commanders",
}

// NFLVerseProvider reads the official injury report from nflverse's daily
// CSV export. It is free and stable but can lag the official report by up to
// a day, so it is only used as a fallback.
type NFLVerseProvider struct {
	client      *http.Client
	injuriesURL string
	gamesURL    string
	now         func() time.Time

	mu        sync.Mutex
	opponents map[string]string
}

func NewNFLVerseProvider(client *http.Client) *NFLVerseProvider {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &NFLVerseProvider{
		client:      client,
		injuriesURL: nflverseInjuriesURL,
		gamesURL:    nflverseGamesURL,
		now:         time.Now,
		opponents:   map[string]string{},
	}
}

func (p *NFLVerseProvider) Fetch(ctx context.Context, request ReportRequest) (Report, error) {
	log.Printf("component=injury operation=fetch provider=nflverse team=%s season=%d week=%d", request.Team, request.Season, request.Week)
	sourceURL := fmt.Sprintf(p.injuriesURL, request.Season)
	rows, lastModified, err := p.readCSV(ctx, sourceURL)
	if err != nil {
		return Report{}, fmt.Errorf("nflverse injuries: %w", err)
	}

	report := Report{
		Season:      request.Season,
		SeasonType:  request.SeasonType,
		Week:        request.Week,
		Team:        request.Team,
		Source:      SourceNFLVerse,
		SourceURL:   sourceURL,
		RetrievedAt: dataTime(lastModified, p.now()),
		Players:     nflversePlayers(rows, request, string(request.Team)),
	}
	sortPlayers(report.Players)
	if err := ValidateReport(report); err != nil {
		return Report{}, fmt.Errorf("invalid nflverse injury report: %w", err)
	}

	opponent, err := p.opponent(ctx, request)
	if err != nil {
		// Publish Miami's report even if the schedule is unavailable.
		log.Printf("component=injury operation=nflverse_opponent error=%v", err)
		return report, nil
	}
	if name, ok := nflverseTeamNames[opponent]; ok {
		players := nflversePlayers(rows, request, opponent)
		sortPlayers(players)
		if err := ValidatePlayers(players); err != nil {
			log.Printf("component=injury operation=parse_opponent opponent=%q error=%v", name, err)
		} else {
			report.OpponentName = name
			report.OpponentPlayers = players
		}
	}
	return report, nil
}

func nflversePlayers(rows []map[string]string, request ReportRequest, team string) []PlayerReport {
	week := strconv.Itoa(request.Week)
	byID := map[string]PlayerReport{}
	var order []string
	for _, row := range rows {
		if row["season"] != strconv.Itoa(request.Season) || row["season_type"] != string(request.SeasonType) ||
			row["week"] != week || row["team"] != team {
			continue
		}
		injury := joinInjuries(row["practice_primary_injury"], row["practice_secondary_injury"])
		if injury == "" {
			injury = joinInjuries(row["report_primary_injury"], row["report_secondary_injury"])
		}
		p := player(row["full_name"], row["position"], injury,
			NormalizePracticeStatus(row["practice_status"]), NormalizeGameStatus(row["report_status"]))
		if _, seen := byID[p.ID]; !seen {
			order = append(order, p.ID)
		}
		byID[p.ID] = p
	}
	players := make([]PlayerReport, 0, len(order))
	for _, id := range order {
		players = append(players, byID[id])
	}
	return players
}

func joinInjuries(primary, secondary string) string {
	primary, secondary = strings.TrimSpace(primary), strings.TrimSpace(secondary)
	if primary != "" && secondary != "" {
		return primary + " / " + secondary
	}
	return primary + secondary
}

// opponent returns the abbreviation of the team's opponent for the week,
// caching it because the schedule file is large.
func (p *NFLVerseProvider) opponent(ctx context.Context, request ReportRequest) (string, error) {
	key := fmt.Sprintf("%d/%s/%d/%s", request.Season, request.SeasonType, request.Week, request.Team)
	p.mu.Lock()
	cached, ok := p.opponents[key]
	p.mu.Unlock()
	if ok {
		return cached, nil
	}

	rows, _, err := p.readCSV(ctx, p.gamesURL)
	if err != nil {
		return "", fmt.Errorf("nflverse schedule: %w", err)
	}
	team := string(request.Team)
	for _, row := range rows {
		if row["season"] != strconv.Itoa(request.Season) || row["week"] != strconv.Itoa(request.Week) {
			continue
		}
		if (request.SeasonType == SeasonTypeREG) != (row["game_type"] == "REG") {
			continue
		}
		var opp string
		switch team {
		case row["home_team"]:
			opp = row["away_team"]
		case row["away_team"]:
			opp = row["home_team"]
		default:
			continue
		}
		p.mu.Lock()
		p.opponents[key] = opp
		p.mu.Unlock()
		return opp, nil
	}
	return "", fmt.Errorf("no %s game in season %d week %d", team, request.Season, request.Week)
}

// dataTime returns when nflverse generated the file, so the monitor can tell
// whether it is older than the last published snapshot.
func dataTime(lastModified string, fallback time.Time) time.Time {
	if t, err := http.ParseTime(lastModified); err == nil {
		return t
	}
	return fallback
}

// readCSV returns the rows keyed by header plus the Last-Modified header.
func (p *NFLVerseProvider) readCSV(ctx context.Context, url string) ([]map[string]string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %d for %s", resp.StatusCode, url)
	}
	lastModified := resp.Header.Get("Last-Modified")

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, "", fmt.Errorf("read header: %w", err)
	}
	var rows []map[string]string
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read row: %w", err)
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(record) {
				row[name] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, lastModified, nil
}

// FallbackProvider tries each provider in order and returns the first report
// that is fetched without error.
type FallbackProvider struct {
	providers []InjuryProvider
}

func NewFallbackProvider(providers ...InjuryProvider) *FallbackProvider {
	return &FallbackProvider{providers: providers}
}

// NewDefaultProvider reads the report from nflverse's open data.
func NewDefaultProvider() *FallbackProvider {
	return NewFallbackProvider(NewNFLVerseProvider(nil))
}

func (f *FallbackProvider) Fetch(ctx context.Context, request ReportRequest) (Report, error) {
	var errs []string
	for i, provider := range f.providers {
		report, err := provider.Fetch(ctx, request)
		if err == nil {
			return report, nil
		}
		errs = append(errs, err.Error())
		if i+1 < len(f.providers) {
			log.Printf("component=injury operation=fallback failed_provider=%d reason=%q", i, err)
		}
	}
	return Report{}, fmt.Errorf("all injury sources failed: %s", strings.Join(errs, "; "))
}
