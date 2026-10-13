package espn

import (
	"log"

	"github.com/Mundo-Dolphins/dolphins-alerts/telegram"
)

var sendGameMessage = telegram.SendHTMLMessage

// StartESPNMonitoring starts ESPN monitoring with built-in callbacks
func StartESPNMonitoring(cacheFile string, telegramBotToken, telegramChatID string) *GameMonitor {
	callbacks := GameCallbacks{
		OnGameStart:    handleGameStart,
		OnGameEnd:      handleGameEnd,
		OnScoring:      handleScoring,
		OnStatusChange: handleStatusChange,
	}

	monitor := NewGameMonitor(cacheFile, callbacks, telegramBotToken, telegramChatID)
	return monitor
}

// handleGameStart handles game start events
func handleGameStart(game GameCache, telegramBotToken, telegramChatID string) error {
	// Telegram message intentionally in Spanish
	message := FormatGameMessage(game)
	err := sendGameMessage(telegramBotToken, telegramChatID, message)
	if err != nil {
		log.Printf("Error sending start notification: %v", err)
		return err
	}
	log.Printf("✅ Start notification sent for game %s", game.ID)
	return nil
}

// handleGameEnd handles game end events
func handleGameEnd(game GameCache, telegramBotToken, telegramChatID string) error {
	// Telegram message intentionally in Spanish
	message := FormatGameEndMessage(game)
	err := sendGameMessage(telegramBotToken, telegramChatID, message)
	if err != nil {
		log.Printf("Error sending end notification: %v", err)
		return err
	}
	log.Printf("✅ End notification sent for game %s", game.ID)
	return nil
}

// handleScoring handles scoring events
func handleScoring(game GameCache, play ScoringPlay, homeScore, awayScore string, telegramBotToken, telegramChatID string) error {
	// Telegram message intentionally in Spanish
	message := FormatScoringMessage(game, play, homeScore, awayScore)
	err := sendGameMessage(telegramBotToken, telegramChatID, message)
	if err != nil {
		log.Printf("Error sending scoring notification: %v", err)
		return err
	}
	log.Printf("✅ Scoring notification sent for game %s: %s", game.ID, play.Text)
	return nil
}

// handleStatusChange handles game status changes
func handleStatusChange(game GameCache, oldStatus, newStatus string, telegramBotToken, telegramChatID string) error {
	// Start and end have dedicated notifications; do not announce them twice.
	if newStatus == StatusInProgress || newStatus == StatusFinal {
		return nil
	}
	message := "ℹ️ " + matchup(game) + "\n\nEstado: " + translatedGameStatus(newStatus)
	err := sendGameMessage(telegramBotToken, telegramChatID, message)
	if err != nil {
		log.Printf("Error sending status update: %v", err)
		return err
	}
	log.Printf("✅ Status update sent for game %s: %s → %s",
		game.ID, oldStatus, newStatus)
	return nil
}

func translatedGameStatus(status string) string {
	switch status {
	case StatusScheduled:
		return "Programado"
	case StatusDelayed:
		return "Retrasado"
	case StatusPostponed:
		return "Aplazado"
	case StatusCanceled:
		return "Cancelado"
	default:
		return "Actualización pendiente de confirmar"
	}
}
