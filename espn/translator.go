package espn

import (
	"fmt"
	"html"
	"strings"
)

// fahrenheitToCelsius converts temperature from Fahrenheit to Celsius
func fahrenheitToCelsius(f int) int {
	return int((float64(f) - 32) * 5 / 9)
}

// TranslateWeather translates weather information into Spanish
func TranslateWeather(weather *Weather) string {
	if weather == nil || weather.DisplayValue == "" {
		return "Información del clima no disponible"
	}

	// Translate common weather conditions
	translated := translateWeatherConditions(weather.DisplayValue)

	// Si tiene temperatura en el campo Temperature, añadir conversión
	if weather.Temperature > 0 {
		celsius := fahrenheitToCelsius(weather.Temperature)
		tempInfo := fmt.Sprintf("(%d°F / %d°C)", weather.Temperature, celsius)

		// If the translation already contains temperature, replace it
		if strings.Contains(translated, "°") {
			// Buscar y reemplazar la parte de temperatura existente
			parts := strings.Fields(translated)
			var result []string
			for _, part := range parts {
				if strings.Contains(part, "°") {
					// Reemplazar con nueva información de temperatura
					result = append(result, tempInfo)
				} else {
					result = append(result, part)
				}
			}
			return strings.Join(result, " ")
		} else {
			// Add temperature at the end
			return translated + " " + tempInfo
		}
	}

	return translated
}

// translateWeatherConditions translates specific weather conditions
func translateWeatherConditions(condition string) string {
	originalCondition := condition
	condition = strings.ToLower(condition)

	// Si contiene temperatura, extraerla y formatearla primero
	if strings.Contains(condition, "°") {
		return formatTemperature(originalCondition)
	}

	// Mapeo de frases completas primero (más específicas)
	phraseTranslations := map[string]string{
		"partly cloudy": "Parcialmente nublado",
		"light rain":    "Lluvia ligera",
		"heavy rain":    "Lluvia fuerte",
		"light snow":    "Nieve ligera",
		"heavy snow":    "Nieve fuerte",
	}

	// Search for full-phrase translations first
	for eng, esp := range phraseTranslations {
		if strings.Contains(condition, eng) {
			return esp
		}
	}

	// Mapping of individual condition words
	wordTranslations := map[string]string{
		"sunny":         "Soleado",
		"clear":         "Despejado",
		"cloudy":        "Nublado",
		"overcast":      "Nublado",
		"rain":          "Lluvia",
		"drizzle":       "Llovizna",
		"showers":       "Chubascos",
		"thunderstorms": "Tormentas",
		"snow":          "Nieve",
		"sleet":         "Aguanieve",
		"fog":           "Niebla",
		"mist":          "Neblina",
		"haze":          "Bruma",
		"windy":         "Ventoso",
		"breezy":        "Ventoso",
		"calm":          "Tranquilo",
		"hot":           "Caluroso",
		"warm":          "Cálido",
		"cool":          "Fresco",
		"cold":          "Frío",
		"freezing":      "Helado",
		"humid":         "Húmedo",
		"dry":           "Seco",
		"fair":          "Despejado",
		"pleasant":      "Agradable",
	}

	// Search for individual word translations
	for eng, esp := range wordTranslations {
		if strings.Contains(condition, eng) {
			return esp
		}
	}

	// If no translation is found, return the original with capitalized first letter
	if len(originalCondition) > 0 {
		return strings.ToUpper(originalCondition[:1]) + originalCondition[1:]
	}

	return originalCondition
}

// formatTemperature formats the temperature string while keeping the degree symbol
func formatTemperature(condition string) string {
	// Keep the original temperature tokens if they include the degree symbol
	parts := strings.Fields(condition)
	var result []string

	for _, part := range parts {
		if strings.Contains(part, "°") {
			result = append(result, part)
		} else {
			// Look for a translation for this token
			partLower := strings.ToLower(part)

			// Handle compound phrases first
			if partLower == "partly" && len(result) == 0 {
				// If "partly" is at the start, it's probably "partly cloudy"
				result = append(result, "Parcialmente")
				continue
			}

			translations := map[string]string{
				"sunny":  "Soleado",
				"clear":  "Despejado",
				"cloudy": "nublado",
				"rain":   "Lluvia",
				"light":  "ligera",
				"heavy":  "fuerte",
			}

			if translated, exists := translations[partLower]; exists {
				result = append(result, translated)
			} else {
				result = append(result, part)
			}
		}
	}

	return strings.Join(result, " ")
}

// TranslateScoringType translates the scoring type to Spanish
func TranslateScoringType(scoringType string) string {
	translations := map[string]string{
		"Touchdown":            "Touchdown",
		"Field Goal":           "Gol de campo",
		"Safety":               "Safety",
		"Extra Point":          "Punto extra",
		"Two Point Conversion": "Conversión de dos puntos",
		"Fumble Recovery":      "Recuperación de balón suelto",
		"Interception Return":  "Devolución de intercepción",
		"Kickoff Return":       "Devolución de patada inicial",
		"Punt Return":          "Devolución de despeje",
		"Blocked Punt":         "Despeje bloqueado",
		"Blocked Field Goal":   "Gol de campo bloqueado",
		"Defensive TD":         "Touchdown defensivo",
		"Pick Six":             "Pick Six",
		"Fumble TD":            "Touchdown por balón suelto",
		"Kick Return TD":       "Touchdown en devolución de patada",
		"Punt Return TD":       "Touchdown en devolución de despeje",
	}

	if translated, exists := translations[scoringType]; exists {
		return translated
	}

	return scoringType
}

// FormatGameMessage formats the game start message
func FormatGameMessage(game GameCache) string {
	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("🏈 El partido entre %s y %s ha comenzado.\n\n", html.EscapeString(competitorName(game.AwayTeam, "el equipo visitante")), html.EscapeString(competitorName(game.HomeTeam, "el equipo local"))))

	// Teams information
	if game.AwayTeam != nil && game.HomeTeam != nil {
		awayEmoji := getTeamEmoji(game.AwayTeam.Team.DisplayName)
		homeEmoji := getTeamEmoji(game.HomeTeam.Team.DisplayName)

		msg.WriteString(fmt.Sprintf("🆚 %s%s @ %s%s\n",
			awayEmoji, html.EscapeString(competitorName(game.AwayTeam, "Visitante")),
			homeEmoji, html.EscapeString(competitorName(game.HomeTeam, "Local"))))

		// Teams' records
		if len(game.AwayTeam.Record) > 0 {
			msg.WriteString(fmt.Sprintf("📊 %s%s (%s)\n",
				awayEmoji, html.EscapeString(competitorName(game.AwayTeam, "Visitante")),
				html.EscapeString(game.AwayTeam.Record[0].Summary)))
		}
		if len(game.HomeTeam.Record) > 0 {
			msg.WriteString(fmt.Sprintf("📊 %s%s (%s)\n",
				homeEmoji, html.EscapeString(competitorName(game.HomeTeam, "Local")),
				html.EscapeString(game.HomeTeam.Record[0].Summary)))
		}
	}

	// Venue information
	if game.Venue != nil {
		msg.WriteString(fmt.Sprintf("🏟️ %s", html.EscapeString(game.Venue.FullName)))
		if game.Venue.Address.City != "" && game.Venue.Address.State != "" {
			msg.WriteString(fmt.Sprintf(", %s, %s", html.EscapeString(game.Venue.Address.City), html.EscapeString(game.Venue.Address.State)))
		}
		msg.WriteString("\n")
	}

	// Weather information
	if game.Weather != nil {
		weatherTranslated := TranslateWeather(game.Weather)
		msg.WriteString(fmt.Sprintf("🌤️ Clima: %s\n", html.EscapeString(weatherTranslated)))
	}

	return msg.String()
}

// competitorName uses the available ESPN labels without inventing a team.
func competitorName(competitor *Competitor, fallback string) string {
	if competitor == nil {
		return fallback
	}
	return teamName(competitor.Team, fallback)
}

func teamName(team Team, fallback string) string {
	for _, name := range []string{team.DisplayName, team.ShortDisplayName, team.Abbreviation, team.Name} {
		if strings.TrimSpace(name) != "" {
			return name
		}
	}
	return fallback
}

func matchup(game GameCache) string {
	return html.EscapeString(competitorName(game.AwayTeam, "Visitante")) + " vs " + html.EscapeString(competitorName(game.HomeTeam, "Local"))
}

func scoreLine(game GameCache, homeScore, awayScore string) string {
	if homeScore == "" || awayScore == "" {
		return ""
	}
	return fmt.Sprintf("📊 Marcador: %s %s - %s %s", html.EscapeString(competitorName(game.AwayTeam, "Visitante")), html.EscapeString(awayScore), html.EscapeString(competitorName(game.HomeTeam, "Local")), html.EscapeString(homeScore))
}

// FormatGameEndMessage never reuses the start announcement. The result is hidden.
func FormatGameEndMessage(game GameCache) string {
	message := "🏁 El partido entre " + html.EscapeString(competitorName(game.AwayTeam, "el equipo visitante")) + " y " + html.EscapeString(competitorName(game.HomeTeam, "el equipo local")) + " ha terminado."
	if game.HomeTeam != nil && game.AwayTeam != nil {
		if score := scoreLine(game, game.HomeTeam.Score, game.AwayTeam.Score); score != "" {
			message += "\n\n<tg-spoiler>" + score + "</tg-spoiler>"
		}
	}
	return message
}

// FormatScoringMessage exposes only the event and matchup; every play detail
// and the single, team-labelled score line are inside a native Telegram spoiler.
func FormatScoringMessage(game GameCache, play ScoringPlay, homeScore, awayScore string) string {
	heading := "🎯 ¡ANOTACIÓN!"
	if play.Type.Name == "score_correction" {
		heading = "📊 Marcador actualizado"
	}
	var details []string
	if name := teamName(play.Team, ""); name != "" {
		details = append(details, "⭐ Anota "+html.EscapeString(name))
	}
	if kind := TranslateScoringType(play.Type.DisplayName); kind != "" && kind != "Anotación" && play.Type.Name != "score_correction" {
		details = append(details, html.EscapeString(kind))
	}
	if play.Text != "" {
		details = append(details, html.EscapeString(play.Text))
	}
	if play.Clock.DisplayValue != "" && play.Period.DisplayName != "" {
		details = append(details, "⏰ "+html.EscapeString(play.Period.DisplayName)+" - "+html.EscapeString(play.Clock.DisplayValue))
	}
	if score := scoreLine(game, homeScore, awayScore); score != "" {
		details = append(details, score)
	}
	message := heading + "\n\n🆚 " + matchup(game)
	if len(details) > 0 {
		message += "\n\n<tg-spoiler>" + strings.Join(details, "\n") + "</tg-spoiler>"
	}
	return message
}

// getTeamEmoji returns the corresponding emoji for the NFL team
// TeamEmoji returns the emoji used for an NFL team, or 🏈 when unknown.
func TeamEmoji(teamName string) string {
	return strings.TrimSpace(getTeamEmoji(teamName))
}

func getTeamEmoji(teamName string) string {
	// Mapa de equipos de la NFL con sus emojis Unicode
	teamEmojis := map[string]string{
		// AFC East
		"Miami Dolphins":       "🐬",
		"Buffalo Bills":        "🦬",
		"New England Patriots": "🇺🇸",
		"New York Jets":        "✈️",

		// AFC North
		"Baltimore Ravens":    "🐦‍⬛",
		"Cincinnati Bengals":  "🐅",
		"Cleveland Browns":    "🤎",
		"Pittsburgh Steelers": "⚫",

		// AFC South
		"Houston Texans":       "🤠",
		"Indianapolis Colts":   "🐎",
		"Jacksonville Jaguars": "🐆",
		"Tennessee Titans":     "⚔️",

		// AFC West
		"Denver Broncos":       "🐴",
		"Kansas City Chiefs":   "🏈",
		"Las Vegas Raiders":    "☠️",
		"Los Angeles Chargers": "⚡",

		// NFC East
		"Dallas Cowboys":        "🤠",
		"New York Giants":       "🗽",
		"Philadelphia Eagles":   "🦅",
		"Washington Commanders": "🏛️",

		// NFC North
		"Chicago Bears":     "🐻",
		"Detroit Lions":     "🦁",
		"Green Bay Packers": "🧀",
		"Minnesota Vikings": "⚔️",

		// NFC South
		"Atlanta Falcons":      "🦅",
		"Carolina Panthers":    "🐾",
		"New Orleans Saints":   "⚜️",
		"Tampa Bay Buccaneers": "☠️",

		// NFC West
		"Arizona Cardinals":   "🐦",
		"Los Angeles Rams":    "🐏",
		"San Francisco 49ers": "⛏️",
		"Seattle Seahawks":    "🦅",
	}

	if emoji, exists := teamEmojis[teamName]; exists {
		return emoji + " "
	}
	return "🏈 " // Emoji por defecto si no se encuentra el equipo
}
