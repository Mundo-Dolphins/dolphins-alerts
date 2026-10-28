package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Mundo-Dolphins/dolphins-alerts/bluesky"
	"github.com/Mundo-Dolphins/dolphins-alerts/config"
	"github.com/Mundo-Dolphins/dolphins-alerts/espn"
	"github.com/Mundo-Dolphins/dolphins-alerts/injury"
	"github.com/Mundo-Dolphins/dolphins-alerts/socialfeed"
)

// App runs the long-lived monitors (ESPN game alerts, injury report, RSS to
// Bluesky mirror) configured by cfg.
type App struct {
	cfg            config.Config
	injuryProvider injury.InjuryProvider
}

// Option customises an App.
type Option func(*App)

// WithInjuryProvider replaces the default injury source (nflverse).
func WithInjuryProvider(p injury.InjuryProvider) Option {
	return func(a *App) { a.injuryProvider = p }
}

func New(cfg config.Config, opts ...Option) *App {
	a := &App{cfg: cfg}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *App) Run(ctx context.Context) {
	bluesky.BlueskyHandle = a.cfg.Bluesky.Handle
	bluesky.BlueskyAppPass = a.cfg.Bluesky.AppPassword
	log.Printf("component=bluesky operation=configure handle=%s", a.cfg.Bluesky.Handle)

	var wg sync.WaitGroup
	a.startTwitter(ctx, &wg)
	a.startESPN()
	a.startInjury()

	<-ctx.Done()
	log.Printf("component=application operation=shutdown reason=%v", ctx.Err())
	wg.Wait()
}

func (a *App) startTwitter(ctx context.Context, wg *sync.WaitGroup) {
	if a.cfg.Feed.Target == "" {
		log.Println("component=socialfeed operation=start enabled=false reason=missing_target")
		return
	}

	source := socialfeed.NewSource(a.cfg.Feed.BaseURL, nil)
	seen := socialfeed.NewSeenStore(a.cfg.Feed.SeenFile)
	if err := seen.Load(); err != nil {
		log.Printf("component=socialfeed operation=load_state error=%v", err)
	}
	log.Printf("component=socialfeed operation=start enabled=true target=@%s rss_base_url=%s", a.cfg.Feed.Target, a.cfg.Feed.BaseURL)

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.runTwitterMonitor(ctx, source, seen)
	}()
}

func (a *App) runTwitterMonitor(ctx context.Context, source *socialfeed.Source, seen *socialfeed.SeenStore) {
	ticker := time.NewTicker(a.cfg.Feed.PollInterval)
	defer ticker.Stop()

	a.checkTwitter(ctx, source, seen)
	for {
		select {
		case <-ctx.Done():
			log.Println("component=socialfeed operation=stop")
			return
		case <-ticker.C:
			a.checkTwitter(ctx, source, seen)
		}
	}
}

func (a *App) checkTwitter(ctx context.Context, source *socialfeed.Source, seen *socialfeed.SeenStore) {
	posts, err := source.Fetch(ctx, a.cfg.Feed.Target)
	if err != nil {
		log.Printf("component=socialfeed operation=fetch error=%v", err)
		return
	}

	newPosts := socialfeed.NewPosts(posts, seen)
	log.Printf("component=socialfeed operation=fetch items=%d new_items=%d", len(posts), len(newPosts))
	stateChanged := false
	for _, post := range newPosts {
		log.Printf("component=socialfeed operation=publish id=%s", post.ID)
		if err := bluesky.PostToBluesky(ctx, post.Text, post.Images, nil); err != nil {
			log.Printf("component=socialfeed operation=publish id=%s error=%v", post.ID, err)
			continue
		}
		seen.Mark(post.ID)
		stateChanged = true
		log.Printf("component=socialfeed operation=publish id=%s status=success", post.ID)
	}
	if stateChanged {
		if err := seen.Save(); err != nil {
			log.Printf("component=socialfeed operation=save_state error=%v", err)
		}
	}
}

func (a *App) startESPN() {
	go func() {
		log.Printf("component=espn operation=start cache_file=%s", a.cfg.ESPN.CacheFile)
		monitor := espn.StartESPNMonitoring(a.cfg.ESPN.CacheFile, a.cfg.Telegram.BotToken, a.cfg.Telegram.SelectedChatID)
		monitor.Start()
	}()
}

func (a *App) startInjury() {
	log.Printf("component=injury operation=start cache_file=%s", a.cfg.Injury.CacheFile)
	monitor := injury.StartInjuryMonitoring(
		a.cfg.Injury.CacheFile,
		a.cfg.Telegram.BotToken,
		a.cfg.Telegram.SelectedChatID,
		a.injuryProvider,
	)
	_ = monitor
}
