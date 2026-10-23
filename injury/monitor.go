package injury

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/Mundo-Dolphins/dolphins-alerts/espn"
	"github.com/Mundo-Dolphins/dolphins-alerts/telegram"
)

//go:embed translations_es.json
var translationsJSON []byte

var (
	translations      map[string]string
	translationsKeys  []string
	translationsOnce  sync.Once
	reTranslations    *regexp.Regexp
	lowerTranslations map[string]string
)

type Publisher interface {
	Publish(ctx context.Context, text string) error
}

type TelegramPublisher struct {
	BotToken string
	ChatID   string
	Sender   telegram.Sender
}

func (p TelegramPublisher) Publish(_ context.Context, text string) error {
	sender := p.Sender
	if sender == nil {
		sender = telegram.DefaultSender
	}
	return sender.Send(p.BotToken, p.ChatID, text)
}

type Game struct {
	Team         TeamCode
	Opponent     TeamCode
	Start        time.Time
	OpponentName string
	Season       int
	SeasonType   SeasonType
	Week         int
}

type ScheduleProvider interface {
	NextGame(ctx context.Context, team TeamCode) (Game, error)
}

type Monitor struct {
	cacheFile                     string
	telegramBotToken              string
	telegramChatID                string
	AllowNotifyOutsidePublishDays bool
	provider                      InjuryProvider
	store                         ReportStore
	schedule                      ScheduleProvider
	telegramRenderer              TelegramRenderer
	telegramPub                   Publisher
	espnClient                    espn.ClientAPI
	team                          TeamCode
	season                        int
	seasonType                    SeasonType
	pollActive                    time.Duration
	pollIdle                      time.Duration
	stopChan                      chan bool
	now                           func() time.Time
	// pending is a changed report that has been seen once and is waiting to be
	// confirmed by a following check, or to be flushed when the window closes.
	pending *pendingReport
}

type pendingReport struct {
	report  Report
	changes ReportChanges
	eventID string
}

// NewMonitor builds a monitor that reads the report from nflverse's open data.
// Use SetProvider to read from another source.
func NewMonitor(cacheFile, telegramBotToken, telegramChatID string) *Monitor {
	return &Monitor{
		provider:         NewDefaultProvider(),
		cacheFile:        cacheFile,
		telegramBotToken: telegramBotToken,
		telegramChatID:   telegramChatID,
		store:            NewJSONReportStore(cacheFile),
		schedule:         &ESPNScheduleProvider{client: espn.NewClient()},
		telegramRenderer: TelegramRenderer{},
		telegramPub:      TelegramPublisher{BotToken: telegramBotToken, ChatID: telegramChatID},
		team:             TeamMIA,
		season:           inferSeason(time.Now()),
		seasonType:       SeasonTypeREG,
		pollActive:       15 * time.Minute,
		pollIdle:         6 * time.Hour,
		stopChan:         make(chan bool),
	}
}

func (m *Monitor) Start() {
	log.Printf("component=injury operation=start version=v2 team=%s", m.team)
	ctx := context.Background()
	for {
		interval := m.pollInterval(ctx)
		_, _ = m.Check(ctx, false, false)
		select {
		case <-m.stopChan:
			log.Printf("component=injury operation=stop")
			return
		case <-time.After(interval):
		}
	}
}

func (m *Monitor) Stop() {
	close(m.stopChan)
}

func (m *Monitor) Check(ctx context.Context, force bool, dryRun bool) (ReportChanges, error) {
	now := m.currentTime()
	var nextGame Game
	if !force && !dryRun && !m.AllowNotifyOutsidePublishDays {
		game, err := m.schedule.NextGame(ctx, m.team)
		if err != nil {
			log.Printf("component=injury operation=schedule error=%v", err)
			return ReportChanges{}, err
		}
		nextGame = game
		if !inPublishWindow(now, game) {
			if m.pending != nil {
				return m.flushPending(ctx)
			}
			log.Printf("component=injury operation=check status=skipped reason=outside_publish_window")
			return ReportChanges{}, nil
		}
	}
	req := ReportRequest{Team: m.team, Season: inferSeason(now), SeasonType: m.seasonType, Week: inferWeek(now), ForceRefresh: force}
	if !nextGame.Start.IsZero() {
		// Keep one snapshot key across all publication dates for this game.
		req.Season = nextGame.Season
		if req.Season == 0 {
			req.Season = inferSeason(nextGame.Start)
		}
		req.Week = nextGame.Week
		if req.Week == 0 {
			req.Week = inferWeek(nextGame.Start)
		}
		if nextGame.SeasonType != "" {
			req.SeasonType = nextGame.SeasonType
		}
	}
	report, err := m.provider.Fetch(ctx, req)
	if err != nil {
		log.Printf("component=injury operation=fetch error=%v", err)
		return ReportChanges{}, err
	}
	if nextGame.OpponentName != "" && report.OpponentName != "" && !strings.EqualFold(strings.TrimSpace(report.OpponentName), strings.TrimSpace(nextGame.OpponentName)) {
		log.Printf("component=injury operation=check status=skipped reason=wrong_opponent expected=%q actual=%q", nextGame.OpponentName, report.OpponentName)
		return ReportChanges{}, nil
	}
	if m.store == nil && m.cacheFile != "" {
		m.store = NewJSONReportStore(m.cacheFile)
	}
	if len(report.Players) == 0 && len(report.OpponentPlayers) == 0 {
		if !dryRun {
			if err := m.store.SaveReport(report); err != nil {
				return ReportChanges{}, err
			}
		}
		log.Printf("component=injury operation=diff players=0 changes=0")
		return ReportChanges{}, nil
	}
	previous, err := m.store.LastReport(report.SnapshotKey())
	if err != nil {
		log.Printf("component=injury operation=load_state error=%v", err)
		return ReportChanges{}, err
	}
	if staleFallback(previous, report) {
		log.Printf("component=injury operation=diff status=skipped reason=stale_fallback data_time=%s snapshot_time=%s",
			report.RetrievedAt.Format(time.RFC3339), previous.RetrievedAt.Format(time.RFC3339))
		return ReportChanges{}, nil
	}
	changes := ReportDiffer{}.Diff(previous, report)
	oppChanges := 0
	if changes.Opponent != nil {
		oppChanges = countChanges(*changes.Opponent)
	}
	log.Printf("component=injury operation=diff players=%d changes=%d opponent=%q opponent_players=%d opponent_changes=%d",
		len(report.Players), countChanges(changes), report.OpponentName, len(report.OpponentPlayers), oppChanges)
	if changes.Empty() && !force {
		// The page is back to what was already published.
		m.pending = nil
		return changes, nil
	}

	eventID := report.SnapshotKey() + ":" + report.Fingerprint()
	if published, err := m.store.WasPublished(eventID); err != nil {
		return changes, err
	} else if published && !force {
		log.Printf("component=injury operation=publish status=skipped reason=already_published event_id=%s", eventID)
		return changes, nil
	}

	// The club publishes the report in several steps (the opponent section
	// often lands later), so only publish once the same report is seen in two
	// consecutive checks. If the window closes first, flushPending publishes it.
	if !force && !dryRun {
		if m.pending == nil || m.pending.report.Fingerprint() != report.Fingerprint() {
			m.pending = &pendingReport{report: report, changes: changes, eventID: eventID}
			log.Printf("component=injury operation=publish status=deferred reason=waiting_for_stable_report event_id=%s", eventID)
			return changes, nil
		}
	}
	if dryRun {
		return changes, nil
	}
	return changes, m.publish(ctx, report, changes, eventID)
}

// flushPending publishes the report deferred by the last in-window check.
func (m *Monitor) flushPending(ctx context.Context) (ReportChanges, error) {
	p := m.pending
	log.Printf("component=injury operation=publish status=flushing reason=window_closed event_id=%s", p.eventID)
	return p.changes, m.publish(ctx, p.report, p.changes, p.eventID)
}

func (m *Monitor) publish(ctx context.Context, report Report, changes ReportChanges, eventID string) error {
	m.pending = nil
	if m.telegramPub != nil {
		log.Printf("component=injury operation=publish publisher=telegram event_id=%s", eventID)
		if err := m.telegramPub.Publish(ctx, m.telegramRenderer.Render(report, changes)); err != nil {
			return err
		}
	}
	if err := m.store.MarkPublished(eventID, time.Now()); err != nil {
		return err
	}
	return m.store.SaveReport(report)
}

func (m *Monitor) pollInterval(ctx context.Context) time.Duration {
	game, err := m.schedule.NextGame(ctx, m.team)
	if err != nil || game.Start.IsZero() {
		return m.pollIdle
	}
	now := m.currentTime()
	if end, ok := publishWindowEnd(now, game); ok {
		// Wake when the window closes so a pending report is not left unpublished.
		if delay := end.Sub(now); m.pending != nil && delay < m.pollActive {
			return delay
		}
		return m.pollActive
	}
	// Wake at 22:00 even if an idle poll would otherwise sleep past it.
	for _, start := range publishWindowStarts(game) {
		if delay := start.Sub(now); delay > 0 && delay < m.pollIdle {
			return delay
		}
	}
	return m.pollIdle
}

// StartInjuryMonitoring starts a monitor in the background. A nil provider
// keeps the default (nflverse).
func StartInjuryMonitoring(cacheFile, telegramBotToken, telegramChatID string, provider InjuryProvider) *Monitor {
	monitor := NewMonitor(cacheFile, telegramBotToken, telegramChatID)
	if provider != nil {
		monitor.SetProvider(provider)
	}
	go monitor.Start()
	return monitor
}

// staleFallback reports whether fallback data was generated before the last
// snapshot taken from the official page, so it cannot be newer and would
// publish false regressions.
func staleFallback(previous *Report, current Report) bool {
	return previous != nil && current.Source == SourceNFLVerse && previous.Source != SourceNFLVerse &&
		!current.RetrievedAt.After(previous.RetrievedAt)
}

func countChanges(c ReportChanges) int {
	return len(c.Added) + len(c.Removed) + len(c.PracticeChanges) + len(c.InjuryChanges) + len(c.GameStatusChanges)
}

func TranslateInjury(s string) string {
	if strings.TrimSpace(s) == "" || strings.EqualFold(s, "-") || strings.EqualFold(s, "N/A") {
		return "No especificado"
	}
	translationsOnce.Do(func() {
		translations = map[string]string{}
		if len(translationsJSON) > 0 {
			_ = json.Unmarshal(translationsJSON, &translations)
		}
		for k := range translations {
			translationsKeys = append(translationsKeys, k)
		}
		sort.Slice(translationsKeys, func(i, j int) bool { return len(translationsKeys[i]) > len(translationsKeys[j]) })
		if len(translationsKeys) > 0 {
			var parts []string
			lowerTranslations = make(map[string]string, len(translations))
			for _, k := range translationsKeys {
				parts = append(parts, regexp.QuoteMeta(k))
				lowerTranslations[strings.ToLower(k)] = translations[k]
			}
			reTranslations = regexp.MustCompile("(?i)(" + strings.Join(parts, "|") + ")")
		}
	})
	segments := strings.Split(s, "/")
	for i, segment := range segments {
		segments[i] = translateSegment(segment)
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.Join(segments, "/"), "  ", " "))
}

var sideRe = regexp.MustCompile(`(?i)^(\s*)(right|left)\s+(.+?)(\s*)$`)

// feminineTerms are the translated body parts that take a feminine adjective.
var feminineTerms = map[string]bool{
	"rodilla": true, "cadera": true, "ingle": true, "pantorrilla": true,
	"espinilla": true, "espalda": true, "cabeza": true, "costilla": true,
	"muñeca": true, "mano": true, "zona lumbar": true,
}

// translateSegment translates one "/"-separated part of an injury. A leading
// "Right"/"Left" becomes a Spanish adjective after the body part, agreeing in
// gender: "Left Knee" -> "rodilla izquierda".
func translateSegment(segment string) string {
	m := sideRe.FindStringSubmatch(segment)
	if m == nil || reTranslations == nil {
		return replaceTranslations(segment)
	}
	lead, side, rest, trail := m[1], strings.ToLower(m[2]), m[3], m[4]
	loc := reTranslations.FindStringIndex(rest)
	if loc == nil || loc[0] != 0 {
		return lead + replaceTranslations(rest) + " (" + sideLabel(side, false) + ")" + trail
	}
	term := lowerTranslations[strings.ToLower(rest[:loc[1]])]
	return lead + term + " " + sideLabel(side, feminineTerms[term]) + replaceTranslations(rest[loc[1]:]) + trail
}

func sideLabel(side string, feminine bool) string {
	switch {
	case side == "right" && feminine:
		return "derecha"
	case side == "right":
		return "derecho"
	case feminine:
		return "izquierda"
	default:
		return "izquierdo"
	}
}

func replaceTranslations(s string) string {
	if reTranslations == nil {
		return s
	}
	return reTranslations.ReplaceAllStringFunc(s, func(match string) string {
		if val, ok := lowerTranslations[strings.ToLower(match)]; ok {
			return val
		}
		return match
	})
}

func (m *Monitor) isNextDolphinsGameOnThursday() (bool, error) {
	if m.espnClient != nil {
		if err := m.espnClient.UpdateCache(); err != nil {
			return false, err
		}
		var next time.Time
		for _, game := range m.espnClient.GetActiveGames() {
			if game.Date.After(time.Now()) && (next.IsZero() || game.Date.Before(next)) {
				next = game.Date
			}
		}
		if next.IsZero() {
			return false, nil
		}
		return next.Weekday() == time.Thursday, nil
	}
	game, err := m.schedule.NextGame(context.Background(), m.team)
	if err != nil {
		return false, err
	}
	return game.Start.Weekday() == time.Thursday, nil
}

func (m *Monitor) SetESPNClient(c espn.ClientAPI) {
	m.espnClient = c
	m.schedule = &ESPNScheduleProvider{client: c}
}

func (m *Monitor) SetProvider(provider InjuryProvider) { m.provider = provider }

func (m *Monitor) SetStore(store ReportStore) { m.store = store }

// SetPublisher replaces the Telegram publisher. The injury report is only
// published to Telegram.
func (m *Monitor) SetPublisher(telegramPub Publisher) {
	m.telegramPub = telegramPub
}
