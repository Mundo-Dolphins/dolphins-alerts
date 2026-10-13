package espn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	scoreboardURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard"
	gameDetailURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard/%s"
	summaryURL    = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/summary?event=%s"

	// Game status constants
	StatusScheduled  = "STATUS_SCHEDULED"
	StatusInProgress = "STATUS_IN_PROGRESS"
	StatusFinal      = "STATUS_FINAL"
	StatusDelayed    = "STATUS_DELAYED"
	StatusPostponed  = "STATUS_POSTPONED"
	StatusCanceled   = "STATUS_CANCELED"

	// Miami Dolphins team ID and abbreviations
	DolphinsAbbreviation = "MIA"
	DolphinsName         = "Dolphins"
)

// Client represents the client for the ESPN API
type Client struct {
	httpClient *http.Client
	cache      *Cache
	// overridable endpoints for testing
	scoreboardURL string
	gameDetailURL string
	summaryURL    string
}

// ClientAPI describes the subset of Client used by other packages for querying
// cached game data. Defined here so callers can mock the ESPN client in tests.
type ClientAPI interface {
	UpdateCache() error
	GetActiveGames() []GameCache
}

// maxResponseBytes bounds how much of an ESPN response is read.
const maxResponseBytes = 8 << 20

// NewClient creates a new instance of the ESPN client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: &Cache{
			Games: make(map[string]GameCache),
		},
		scoreboardURL: scoreboardURL,
		gameDetailURL: gameDetailURL,
		summaryURL:    summaryURL,
	}
}

// upcomingWeeks is how many weeks after the current one GetScoreboard loads,
// roughly two months of schedule.
const upcomingWeeks = 8

// GetScoreboard gets the NFL scoreboard for the current week and the upcoming
// weeks. ESPN rejects date ranges (dates=START-END) with a 400, so the
// schedule is loaded week by week.
func (c *Client) GetScoreboard() (*ScoreboardResponse, error) {
	current, err := c.fetchScoreboard(c.scoreboardURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scoreboard: %w", err)
	}

	seen := make(map[string]bool, len(current.Events))
	for _, ev := range current.Events {
		seen[ev.ID] = true
	}

	seasonType, week := current.Season.Type, current.Week.Number
	for i := 0; i < upcomingWeeks; i++ {
		var ok bool
		seasonType, week, ok = nextScoreboardWeek(seasonType, week)
		if !ok {
			break
		}
		url := fmt.Sprintf("%s?seasontype=%d&week=%d", c.scoreboardURL, seasonType, week)
		next, err := c.fetchScoreboard(url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch scoreboard for season type %d week %d: %w", seasonType, week, err)
		}
		for _, ev := range next.Events {
			if !seen[ev.ID] {
				seen[ev.ID] = true
				current.Events = append(current.Events, ev)
			}
		}
	}
	return current, nil
}

// nextScoreboardWeek returns the week after the given one, moving from
// preseason to regular season to postseason.
func nextScoreboardWeek(seasonType, week int) (int, int, bool) {
	lastWeek := map[int]int{1: 4, 2: 18, 3: 5}
	last, known := lastWeek[seasonType]
	if !known || week < 1 {
		return 0, 0, false
	}
	if week < last {
		return seasonType, week + 1, true
	}
	if seasonType < 3 {
		return seasonType + 1, 1, true
	}
	return 0, 0, false
}

func (c *Client) fetchScoreboard(url string) (*ScoreboardResponse, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read scoreboard response: %w", err)
	}

	var scoreboard ScoreboardResponse
	if err := json.Unmarshal(body, &scoreboard); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scoreboard response: %w", err)
	}
	return &scoreboard, nil
}

// GetScoreboardForDate gets the scoreboard for a specific date (YYYYMMDD)
func (c *Client) GetScoreboardForDate(date string) (*ScoreboardResponse, error) {
	url := fmt.Sprintf("%s?dates=%s", c.scoreboardURL, date)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scoreboard for date %s: %w", date, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status %d for date %s", resp.StatusCode, date)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response for date %s: %w", date, err)
	}

	var scoreboard ScoreboardResponse
	if err := json.Unmarshal(body, &scoreboard); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scoreboard for date %s: %w", date, err)
	}

	return &scoreboard, nil
}

// GetGameDetail gets the details for a specific game
func (c *Client) GetGameDetail(gameID string) (*GameDetailResponse, error) {
	url := fmt.Sprintf(c.gameDetailURL, gameID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch game details for %s: %w", gameID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status %d for game %s", resp.StatusCode, gameID)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response for game %s: %w", gameID, err)
	}

	var gameDetail GameDetailResponse
	if err := json.Unmarshal(body, &gameDetail); err != nil {
		return nil, fmt.Errorf("failed to unmarshal game details for %s: %w", gameID, err)
	}

	return &gameDetail, nil
}

// GetGameSummary gets the summary for a game (including scoring plays)
func (c *Client) GetGameSummary(gameID string) (*SummaryResponse, error) {
	url := fmt.Sprintf(c.summaryURL, gameID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch game summary for %s: %w", gameID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status %d for summary of game %s", resp.StatusCode, gameID)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read summary response for %s: %w", gameID, err)
	}

	var summary SummaryResponse
	if err := json.Unmarshal(body, &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal summary for game %s: %w", gameID, err)
	}

	return &summary, nil
}

// FindDolphinsGames busca los partidos de los Dolphins en el scoreboard
func (c *Client) FindDolphinsGames(scoreboard *ScoreboardResponse) []Event {
	var dolphinsGames []Event

	for _, event := range scoreboard.Events {
		for _, competition := range event.Competitions {
			for _, competitor := range competition.Competitors {
				if competitor.Team.Abbreviation == DolphinsAbbreviation ||
					strings.Contains(strings.ToLower(competitor.Team.Name), strings.ToLower(DolphinsName)) {
					dolphinsGames = append(dolphinsGames, event)
					break
				}
			}
		}
	}

	return dolphinsGames
}

// IsGameActive verifica si un partido está activo (en progreso)
func (c *Client) IsGameActive(status Status) bool {
	return status.Type.Name == StatusInProgress
}

// IsGameScheduled verifica si un partido está programado
func (c *Client) IsGameScheduled(status Status) bool {
	return status.Type.Name == StatusScheduled
}

// IsGameFinished verifica si un partido ha terminado
func (c *Client) IsGameFinished(status Status) bool {
	return status.Type.Name == StatusFinal
}

// ShouldCacheBeUpdated verifica si la caché debe ser actualizada
func (c *Client) ShouldCacheBeUpdated() bool {
	if c.cache.LastUpdated.IsZero() {
		return true
	}

	// Actualizar la caché una vez al día
	return time.Since(c.cache.LastUpdated) > 24*time.Hour
}

// UpdateCache actualiza la caché con los partidos de los Dolphins
func (c *Client) UpdateCache() error {
	var allDolphinsGames []Event

	// Obtener partidos de hoy
	scoreboard, err := c.GetScoreboard()
	if err != nil {
		return fmt.Errorf("error al obtener scoreboard actual: %w", err)
	}
	todayGames := c.FindDolphinsGames(scoreboard)
	allDolphinsGames = append(allDolphinsGames, todayGames...)

	// Obtener partidos de los próximos 14 días
	now := time.Now()
	for i := 1; i <= 14; i++ {
		futureDate := now.AddDate(0, 0, i)
		dateStr := futureDate.Format("20060102") // YYYYMMDD

		futureScoreboard, err := c.GetScoreboardForDate(dateStr)
		if err != nil {
			// No fallar si una fecha específica falla, continuar con las otras
			continue
		}

		futureDolphinsGames := c.FindDolphinsGames(futureScoreboard)
		allDolphinsGames = append(allDolphinsGames, futureDolphinsGames...)
	}

	// Debug: Mostrar cuántos partidos se encontraron
	// fmt.Printf("DEBUG: Found %d total Dolphins games across all dates\n", len(allDolphinsGames))

	// Procesar todos los partidos encontrados
	for _, game := range allDolphinsGames {
		// fmt.Printf("DEBUG: Processing game %s - %s\n", game.ID, game.Name)

		gameDate, err := time.Parse("2006-01-02T15:04Z", game.Date)
		if err != nil {
			// Intentar con formato RFC3339 como fallback
			gameDate, err = time.Parse(time.RFC3339, game.Date)
			if err != nil {
				// fmt.Printf("DEBUG: Error parsing date for game %s (%s): %v\n", game.ID, game.Date, err)
				continue
			}
		}

		// Obtener información adicional del partido
		var weather *Weather
		var venue *Venue
		var homeTeam, awayTeam *Competitor

		if len(game.Competitions) > 0 {
			comp := game.Competitions[0]
			if comp.Weather.DisplayValue != "" {
				weather = &comp.Weather
			}
			venue = &comp.Venue

			for _, competitor := range comp.Competitors {
				if competitor.HomeAway == "home" {
					homeTeam = &competitor
				} else {
					awayTeam = &competitor
				}
			}
		}

		// Actualizar o crear entrada en caché
		gameCache := GameCache{
			ID:               game.ID,
			Season:           game.Season,
			Week:             game.Week,
			Date:             gameDate,
			Status:           game.Status.Type.Name,
			LastChecked:      time.Now(),
			IsActive:         c.IsGameActive(game.Status),
			SeenScoringPlays: make(map[string]bool),
			Weather:          weather,
			Venue:            venue,
			HomeTeam:         homeTeam,
			AwayTeam:         awayTeam,
			LastHomeScore:    0,
			LastAwayScore:    0,
			LastPeriod:       0,
		}

		// Si ya existe, preservar los scoring plays ya vistos
		if existingGame, exists := c.cache.Games[game.ID]; exists {
			gameCache.SeenScoringPlays = existingGame.SeenScoringPlays
		}

		c.cache.Games[game.ID] = gameCache
	}

	c.cache.LastUpdated = time.Now()
	return nil
}

// GetActiveGames devuelve los partidos activos de los Dolphins
func (c *Client) GetActiveGames() []GameCache {
	var activeGames []GameCache

	for _, game := range c.cache.Games {
		if game.IsActive || c.IsGameScheduled(Status{Type: StatusType{Name: game.Status}}) {
			activeGames = append(activeGames, game)
		}
	}

	return activeGames
}

// MarkScoringPlayAsSeen marca un scoring play como ya visto
func (c *Client) MarkScoringPlayAsSeen(gameID, scoringPlayID string) {
	if game, exists := c.cache.Games[gameID]; exists {
		game.SeenScoringPlays[scoringPlayID] = true
		c.cache.Games[gameID] = game
	}
}

// IsScoringPlaySeen verifica si un scoring play ya fue visto
func (c *Client) IsScoringPlaySeen(gameID, scoringPlayID string) bool {
	if game, exists := c.cache.Games[gameID]; exists {
		return game.SeenScoringPlays[scoringPlayID]
	}
	return false
}

// UpdateGameStatus actualiza el estado de un partido en caché
func (c *Client) UpdateGameStatus(gameID string, status Status) {
	if game, exists := c.cache.Games[gameID]; exists {
		game.Status = status.Type.Name
		game.IsActive = c.IsGameActive(status)
		game.LastChecked = time.Now()
		c.cache.Games[gameID] = game
	}
}
