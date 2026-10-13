package espn

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// GameMonitor manages monitoring of Dolphins games
type GameMonitor struct {
	client           *Client
	cacheFile        string
	callbacks        GameCallbacks
	telegramBotToken string
	telegramChatID   string
	activeGame       *GameCache
}

// GameCallbacks defines callback functions for game events
type GameCallbacks struct {
	OnGameStart    func(game GameCache, telegramBotToken, telegramChatID string) error
	OnGameEnd      func(game GameCache, telegramBotToken, telegramChatID string) error
	OnScoring      func(game GameCache, play ScoringPlay, homeScore, awayScore string, telegramBotToken, telegramChatID string) error
	OnQuarterEnd   func(game GameCache, quarter string, telegramBotToken, telegramChatID string) error
	OnStatusChange func(game GameCache, oldStatus, newStatus string, telegramBotToken, telegramChatID string) error
}

// NewGameMonitor creates a new instance of the game monitor
func NewGameMonitor(cacheFile string, callbacks GameCallbacks, telegramBotToken, telegramChatID string) *GameMonitor {
	client := NewClient()

	monitor := &GameMonitor{
		client:           client,
		cacheFile:        cacheFile,
		callbacks:        callbacks,
		telegramBotToken: telegramBotToken,
		telegramChatID:   telegramChatID,
		activeGame:       nil,
	}

	monitor.loadCache()
	return monitor
}

// Start begins monitoring games
func (gm *GameMonitor) Start() {
	log.Println("🏈 Starting ESPN games monitor - Dolphins")
	log.Println("📋 Mode: daily scan for scheduled games")

	nextScan := gm.refreshSchedule()

	// All state belongs to this loop. A separate active-game goroutine used to
	// deadlock while signalling its own stop, blocking subsequent games.
	dailyCheck := time.NewTimer(nextScan)
	defer dailyCheck.Stop()
	gameCheck := time.NewTicker(2 * time.Minute)
	defer gameCheck.Stop()

	gm.pollGames()
	for {
		select {
		case <-dailyCheck.C:
			dailyCheck.Reset(gm.refreshSchedule())
		case <-gameCheck.C:
			gm.pollGames()
		}
	}
}

func (gm *GameMonitor) refreshSchedule() time.Duration {
	if err := gm.loadWeeklyGames(); err != nil {
		log.Printf("Error loading weekly games; retrying in one hour: %v", err)
		return time.Hour
	}
	return untilNextDailyScan(time.Now())
}

func untilNextDailyScan(now time.Time) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}

func (gm *GameMonitor) pollGames() {
	if err := gm.monitorActiveGame(); err != nil {
		log.Printf("Error monitoring active game: %v", err)
	}
	if err := gm.updatePastGames(); err != nil {
		log.Printf("Error checking games: %v", err)
	}
}

func (gm *GameMonitor) loadWeeklyGames() error {
	log.Println("🔄 Fetching upcoming Dolphins games...")

	scoreboard, err := gm.client.GetScoreboard()
	if err != nil {
		return fmt.Errorf("failed to fetch scoreboard: %w", err)
	}

	// Include overnight games as well as the next two months.
	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.AddDate(0, 2, 0) // 2 meses desde ahora
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, time.UTC)

	log.Printf("🗓️  Searching games from: %s to: %s",
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))

	dolphinsGames := 0

	for _, event := range scoreboard.Events {
		// Verificar que hay competidores
		if len(event.Competitions) == 0 {
			continue
		}

		competition := event.Competitions[0]

		// Verificar si Miami Dolphins está en el juego
		var hasDolphins bool
		for _, competitor := range competition.Competitors {
			if competitor.Team.DisplayName == "Miami Dolphins" {
				hasDolphins = true
				break
			}
		}

		if !hasDolphins {
			continue
		}

		// Intentar parsear la fecha del evento (formato RFC3339)
		gameTime, err := time.Parse(time.RFC3339, event.Date)
		if err != nil {
			// Try with Z format
			gameTime, err = time.Parse("2006-01-02T15:04Z", event.Date)
			if err != nil {
				log.Printf("Failed to parse event date: %s, error: %v", event.Date, err)
				continue
			}
		}

		// Restrict discovery to the monitoring horizon.
		if gameTime.Before(startDate) {
			continue
		}
		if gameTime.After(endDate) {
			continue
		}

		dolphinsGames++
		gameCache := gm.eventToGameCache(event)
		if existing, ok := gm.client.cache.Games[event.ID]; ok {
			// Discovery refreshes metadata, never notification progress. Details
			// must observe the transition before the cached status advances.
			gameCache.Status = existing.Status
			gameCache.IsActive = existing.IsActive
			gameCache.LastHomeScore = existing.LastHomeScore
			gameCache.LastAwayScore = existing.LastAwayScore
			gameCache.LastPeriod = existing.LastPeriod
			gameCache.SeenScoringPlays = existing.SeenScoringPlays
		}
		gm.client.cache.Games[event.ID] = gameCache

		log.Printf("📅 Scheduled game: %s (%s) - %s",
			event.ID,
			gameTime.Format("2006-01-02 15:04"),
			event.Name)
	}

	log.Printf("✅ Found %d upcoming Dolphins games", dolphinsGames)

	if err := gm.saveCache(); err != nil {
		log.Printf("Warning: error saving cache: %v", err)
	}

	return nil
}

func (gm *GameMonitor) eventToGameCache(event Event) GameCache {
	gameTime, err := time.Parse(time.RFC3339, event.Date)
	if err != nil {
		// Intentar con formato Z
		gameTime, _ = time.Parse("2006-01-02T15:04Z", event.Date)
	}

	game := GameCache{
		ID:               event.ID,
		Season:           event.Season,
		Week:             event.Week,
		Date:             gameTime,
		Status:           event.Status.Type.Name,
		LastChecked:      time.Now(),
		IsActive:         event.Status.Type.Name == StatusInProgress,
		SeenScoringPlays: make(map[string]bool),
	}
	if len(event.Competitions) > 0 {
		updateGameCompetition(&game, event.Competitions[0])
	}
	return game
}

// Refresh display data without resetting the scores used to detect changes.
func updateGameCompetition(game *GameCache, competition Competition) {
	for _, competitor := range competition.Competitors {
		switch competitor.HomeAway {
		case "home":
			game.HomeTeam = &competitor
		case "away":
			game.AwayTeam = &competitor
		}
	}
	if competition.Venue.FullName != "" {
		game.Venue = &competition.Venue
	}
	if competition.Weather.DisplayValue != "" {
		game.Weather = &competition.Weather
	}
}

// updatePastGames also recovers games found already in progress on startup.
func (gm *GameMonitor) updatePastGames() error {
	now := time.Now()
	for gameID, game := range gm.client.cache.Games {
		if gm.activeGame != nil && gm.activeGame.ID == gameID {
			continue
		}
		switch game.Status {
		case StatusScheduled, StatusDelayed, StatusPostponed, StatusInProgress:
		default:
			continue
		}
		if game.Date.After(now.Add(time.Hour)) {
			continue
		}
		detail, err := gm.client.GetGameDetail(gameID)
		if err != nil {
			log.Printf("Error fetching game details %s: %v", gameID, err)
			continue
		}
		oldStatus := game.Status
		game.Status = detail.Status.Type.Name
		game.IsActive = game.Status == StatusInProgress
		game.LastChecked = now
		if len(detail.Competitions) > 0 {
			updateGameCompetition(&game, detail.Competitions[0])
		}
		if game.Status == StatusInProgress && gm.activeGame == nil {
			if oldStatus != StatusInProgress && gm.callbacks.OnGameStart != nil {
				if err := gm.callbacks.OnGameStart(game, gm.telegramBotToken, gm.telegramChatID); err != nil {
					log.Printf("Error sending start notification: %v", err)
					continue // Retry before committing the transition.
				}
			}
			gm.startActiveGameMonitoring(game)
		} else if game.Status == StatusFinal && oldStatus != StatusFinal && now.Sub(game.Date) <= 8*time.Hour && gm.callbacks.OnGameEnd != nil {
			if err := gm.callbacks.OnGameEnd(game, gm.telegramBotToken, gm.telegramChatID); err != nil {
				log.Printf("Error sending end notification: %v", err)
				continue
			}
		}
		gm.client.cache.Games[gameID] = game
		if err := gm.saveCache(); err != nil {
			return err
		}
	}
	return nil
}

func (gm *GameMonitor) startActiveGameMonitoring(game GameCache) {
	gm.activeGame = &game
	log.Printf("🔥 Monitoring game every two minutes: %s", game.ID)
}

func (gm *GameMonitor) monitorActiveGame() error {
	if gm.activeGame == nil {
		return nil
	}

	log.Printf("🔍 Monitoring game: %s", gm.activeGame.ID)

	gameDetail, err := gm.client.GetGameDetail(gm.activeGame.ID)
	if err != nil {
		return fmt.Errorf("error fetching game details: %w", err)
	}

	oldStatus := gm.activeGame.Status
	newStatus := gameDetail.Status.Type.Name
	if len(gameDetail.Competitions) > 0 {
		updateGameCompetition(gm.activeGame, gameDetail.Competitions[0])
	}

	if newStatus != oldStatus {
		log.Printf("📊 Status change: %s → %s", oldStatus, newStatus)

		gm.activeGame.Status = newStatus
		gm.activeGame.IsActive = newStatus == StatusInProgress
		gm.client.cache.Games[gm.activeGame.ID] = *gm.activeGame

		if gm.callbacks.OnStatusChange != nil {
			if err := gm.callbacks.OnStatusChange(*gm.activeGame, oldStatus, newStatus,
				gm.telegramBotToken, gm.telegramChatID); err != nil {
				log.Printf("Error in status change callback: %v", err)
			}
		}

		if newStatus == StatusFinal {
			log.Printf("🏁 Game finished: %s", gm.activeGame.ID)

			if gm.callbacks.OnGameEnd != nil {
				if err := gm.callbacks.OnGameEnd(*gm.activeGame, gm.telegramBotToken, gm.telegramChatID); err != nil {
					gm.activeGame.Status = oldStatus
					gm.activeGame.IsActive = oldStatus == StatusInProgress
					gm.client.cache.Games[gm.activeGame.ID] = *gm.activeGame
					return fmt.Errorf("game end notification: %w", err)
				}
			}
		}
	}

	if newStatus == StatusFinal || newStatus == StatusCanceled {
		err := gm.saveCache()
		gm.stopActiveGameMonitoring()
		return err
	}

	if err := gm.checkScoringAndQuarters(gameDetail); err != nil {
		return err
	}

	return gm.saveCache()
}

func (gm *GameMonitor) checkScoringAndQuarters(gameDetail *GameDetailResponse) error {
	if gm.activeGame == nil || len(gameDetail.Competitions) == 0 {
		return nil
	}

	competition := gameDetail.Competitions[0]
	updateGameCompetition(gm.activeGame, competition)

	var homeScore, awayScore int
	var currentPeriod int
	var haveHomeScore, haveAwayScore bool

	for _, competitor := range competition.Competitors {
		score, err := strconv.Atoi(competitor.Score)
		if err != nil || score < 0 {
			continue
		}

		if competitor.HomeAway == "home" {
			homeScore = score
			haveHomeScore = true
		} else if competitor.HomeAway == "away" {
			awayScore = score
			haveAwayScore = true
		}
	}
	if !haveHomeScore || !haveAwayScore {
		return nil
	}

	currentPeriod = gameDetail.Status.Period

	if currentPeriod != gm.activeGame.LastPeriod && gm.activeGame.LastPeriod > 0 {
		log.Printf("🕐 Quarter change: %d → %d", gm.activeGame.LastPeriod, currentPeriod)
		if gm.callbacks.OnQuarterEnd != nil {
			quarterName := fmt.Sprintf("Q%d", gm.activeGame.LastPeriod)
			if err := gm.callbacks.OnQuarterEnd(*gm.activeGame, quarterName, gm.telegramBotToken, gm.telegramChatID); err != nil {
				log.Printf("Error in quarter end callback: %v", err)
			}
		}
	}

	if homeScore != gm.activeGame.LastHomeScore || awayScore != gm.activeGame.LastAwayScore {
		log.Printf("🏈 Score change: %d-%d → %d-%d",
			gm.activeGame.LastHomeScore, gm.activeGame.LastAwayScore,
			homeScore, awayScore)

		scoringPlay := ScoringPlay{
			ID: fmt.Sprintf("%s_%d_%d_%d", gm.activeGame.ID, currentPeriod, homeScore, awayScore),
			Type: ScoringType{
				Name:        "scoring",
				DisplayName: "Anotación",
			},
			Period: Period{
				Type:        currentPeriod,
				Number:      currentPeriod,
				DisplayName: fmt.Sprintf("Q%d", currentPeriod),
			},
			Clock: Clock{DisplayValue: gameDetail.Status.DisplayClock},
		}
		homeDelta := homeScore - gm.activeGame.LastHomeScore
		awayDelta := awayScore - gm.activeGame.LastAwayScore
		switch {
		case homeDelta < 0 || awayDelta < 0:
			// A score correction is not a new scoring play.
			scoringPlay.Type = ScoringType{Name: "score_correction"}
		case homeDelta > 0 && awayDelta == 0:
			if gm.activeGame.HomeTeam != nil {
				scoringPlay.Team = gm.activeGame.HomeTeam.Team
			}
			scoringPlay.ScoreValue = homeDelta
		case awayDelta > 0 && homeDelta == 0:
			if gm.activeGame.AwayTeam != nil {
				scoringPlay.Team = gm.activeGame.AwayTeam.Team
			}
			scoringPlay.ScoreValue = awayDelta
		default:
			scoringPlay.Text = "Ambos equipos han sumado puntos desde la última actualización."
		}

		if gm.callbacks.OnScoring != nil {
			homeScoreStr := fmt.Sprintf("%d", homeScore)
			awayScoreStr := fmt.Sprintf("%d", awayScore)
			if err := gm.callbacks.OnScoring(*gm.activeGame, scoringPlay, homeScoreStr, awayScoreStr, gm.telegramBotToken, gm.telegramChatID); err != nil {
				return fmt.Errorf("scoring notification: %w", err)
			}
		}

		gm.activeGame.LastHomeScore = homeScore
		gm.activeGame.LastAwayScore = awayScore
		gm.client.cache.Games[gm.activeGame.ID] = *gm.activeGame
	}

	if currentPeriod != gm.activeGame.LastPeriod {
		gm.activeGame.LastPeriod = currentPeriod
		gm.client.cache.Games[gm.activeGame.ID] = *gm.activeGame
	}

	return nil
}

func (gm *GameMonitor) stopActiveGameMonitoring() {
	gm.activeGame = nil
}

func (gm *GameMonitor) loadCache() {
	if _, err := os.Stat(gm.cacheFile); os.IsNotExist(err) {
		log.Printf("Cache file does not exist, creating new one: %s", gm.cacheFile)
		gm.client.cache = &Cache{Games: make(map[string]GameCache)}
		return
	}

	file, err := os.Open(gm.cacheFile)
	if err != nil {
		log.Printf("Error opening cache file: %v", err)
		gm.client.cache = &Cache{Games: make(map[string]GameCache)}
		return
	}
	defer func() { _ = file.Close() }()

	var cache Cache
	if err := json.NewDecoder(file).Decode(&cache); err != nil {
		log.Printf("Error decoding cache: %v", err)
		gm.client.cache = &Cache{Games: make(map[string]GameCache)}
		return
	}

	if cache.Games == nil {
		cache.Games = make(map[string]GameCache)
	}
	gm.client.cache = &cache
	log.Printf("Cache loaded with %d games", len(cache.Games))
}

func (gm *GameMonitor) saveCache() error {
	if err := os.MkdirAll(filepath.Dir(gm.cacheFile), 0755); err != nil {
		return fmt.Errorf("error creando directorio de caché: %w", err)
	}

	gm.client.cache.LastUpdated = time.Now()

	file, err := os.Create(gm.cacheFile)
	if err != nil {
		return fmt.Errorf("error creando archivo de caché: %w", err)
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(gm.client.cache); err != nil {
		return fmt.Errorf("error codificando caché: %w", err)
	}

	return nil
}
