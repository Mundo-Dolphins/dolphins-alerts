package espn

import "time"

// ScoreboardResponse representa la respuesta de la API de ESPN para el scoreboard
type ScoreboardResponse struct {
	Season ScoreboardSeason `json:"season"`
	Week   ScoreboardWeek   `json:"week"`
	Events []Event          `json:"events"`
}

// ScoreboardSeason identifies the season shown by the scoreboard
// (type 1 preseason, 2 regular season, 3 postseason).
type ScoreboardSeason struct {
	Type int `json:"type"`
	Year int `json:"year"`
}

// ScoreboardWeek is the week shown by the scoreboard
type ScoreboardWeek struct {
	Number int `json:"number"`
}

// Event representa un partido en el scoreboard
type Event struct {
	Season       ScoreboardSeason `json:"season"`
	Week         ScoreboardWeek   `json:"week"`
	ID           string           `json:"id"`
	Date         string           `json:"date"`
	Name         string           `json:"name"`
	ShortName    string           `json:"shortName"`
	Status       Status           `json:"status"`
	Competitions []Competition    `json:"competitions"`
}

// Status representa el estado del partido
type Status struct {
	Clock        float64    `json:"clock"`
	DisplayClock string     `json:"displayClock"`
	Period       int        `json:"period"`
	Type         StatusType `json:"type"`
}

// StatusType representa el tipo de estado del partido
type StatusType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Completed   bool   `json:"completed"`
	Description string `json:"description"`
	Detail      string `json:"detail"`
	ShortDetail string `json:"shortDetail"`
}

// Competition representa la competición del partido
type Competition struct {
	ID          string       `json:"id"`
	Date        string       `json:"date"`
	Attendance  int          `json:"attendance"`
	Venue       Venue        `json:"venue"`
	Competitors []Competitor `json:"competitors"`
	Weather     Weather      `json:"weather,omitempty"`
}

// Venue representa el lugar donde se juega el partido
type Venue struct {
	ID       string  `json:"id"`
	FullName string  `json:"fullName"`
	Address  Address `json:"address"`
}

// Address representa la dirección del venue
type Address struct {
	City  string `json:"city"`
	State string `json:"state"`
}

// Competitor representa un equipo en el partido
type Competitor struct {
	ID       string   `json:"id"`
	UID      string   `json:"uid"`
	Type     string   `json:"type"`
	Order    int      `json:"order"`
	HomeAway string   `json:"homeAway"`
	Team     Team     `json:"team"`
	Score    string   `json:"score"`
	Record   []Record `json:"records"`
}

// Team representa información del equipo
type Team struct {
	ID               string `json:"id"`
	UID              string `json:"uid"`
	Location         string `json:"location"`
	Name             string `json:"name"`
	Abbreviation     string `json:"abbreviation"`
	DisplayName      string `json:"displayName"`
	ShortDisplayName string `json:"shortDisplayName"`
	Color            string `json:"color"`
	AlternateColor   string `json:"alternateColor"`
}

// Record representa el record del equipo
type Record struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Type         string `json:"type"`
	Summary      string `json:"summary"`
}

// Weather representa la información del clima
type Weather struct {
	DisplayValue string `json:"displayValue"`
	Temperature  int    `json:"temperature"`
	ConditionID  string `json:"conditionId"`
}

// GameDetailResponse representa la respuesta de la API para un partido específico
type GameDetailResponse struct {
	Header       Header        `json:"header"`
	Status       Status        `json:"status"`
	Competitions []Competition `json:"competitions"`
}

// Header contiene información general del partido
type Header struct {
	ID          string      `json:"id"`
	Competition Competition `json:"competition"`
}

// SummaryResponse representa la respuesta de la API para el summary del partido
type SummaryResponse struct {
	ScoringPlays []ScoringPlay `json:"scoringPlays"`
	Header       Header        `json:"header"`
}

// ScoringPlay representa una anotación en el partido
type ScoringPlay struct {
	ID         string      `json:"id"`
	Type       ScoringType `json:"type"`
	Text       string      `json:"text"`
	ScoreValue int         `json:"scoreValue"`
	Team       Team        `json:"team"`
	Clock      Clock       `json:"clock"`
	Period     Period      `json:"period"`
	Coordinate Coordinate  `json:"coordinate,omitempty"`
}

// ScoringType representa el tipo de anotación
type ScoringType struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Abbreviation string `json:"abbreviation"`
}

// Clock representa el tiempo del juego
type Clock struct {
	DisplayValue string  `json:"displayValue"`
	Value        float64 `json:"value"`
}

// Period representa el período del juego
type Period struct {
	Type        int    `json:"type"`
	Number      int    `json:"number"`
	DisplayName string `json:"displayName"`
}

// Coordinate representa la posición en el campo
type Coordinate struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// GameCache representa un partido en caché
type GameCache struct {
	Season           ScoreboardSeason `json:"season,omitempty"`
	Week             ScoreboardWeek   `json:"week,omitempty"`
	ID               string           `json:"id"`
	Date             time.Time        `json:"date"`
	Status           string           `json:"status"`
	LastChecked      time.Time        `json:"last_checked"`
	IsActive         bool             `json:"is_active"`
	SeenScoringPlays map[string]bool  `json:"seen_scoring_plays"`
	Weather          *Weather         `json:"weather,omitempty"`
	Venue            *Venue           `json:"venue,omitempty"`
	HomeTeam         *Competitor      `json:"home_team,omitempty"`
	AwayTeam         *Competitor      `json:"away_team,omitempty"`
	LastHomeScore    int              `json:"last_home_score"`
	LastAwayScore    int              `json:"last_away_score"`
	LastPeriod       int              `json:"last_period"`
}

// Cache representa la estructura de caché general
type Cache struct {
	LastUpdated time.Time            `json:"last_updated"`
	Games       map[string]GameCache `json:"games"`
}
