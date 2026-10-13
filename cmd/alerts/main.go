package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Mundo-Dolphins/dolphins-alerts/app"
	"github.com/Mundo-Dolphins/dolphins-alerts/config"
	"github.com/Mundo-Dolphins/dolphins-alerts/injury"
	"github.com/joho/godotenv"
)

func init() {
	// Configurar log para escribir a stdout en lugar de stderr
	log.SetOutput(os.Stdout)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "injury" {
		runInjuryCommand(os.Args[2:])
		return
	}

	var useTest bool
	flag.BoolVar(&useTest, "use-test-channel", false, "Use TELEGRAM_CHANNEL_ID_TEST instead of TELEGRAM_CHANNEL_ID")
	flag.Parse()

	// Primero intentar cargar .env del sistema (tiene prioridad)
	err := godotenv.Load()
	if err != nil {
		log.Println("Could not load .env file, using process environment")
	}

	cfg := config.Load(useTest)
	if cfg.Telegram.UseTestChannel {
		log.Printf("component=telegram operation=select_channel source=TELEGRAM_CHANNEL_ID_TEST")
	} else {
		log.Printf("component=telegram operation=select_channel source=TELEGRAM_CHANNEL_ID")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app.New(cfg).Run(ctx)
}

func runInjuryCommand(args []string) {
	if len(args) == 0 || args[0] != "check" {
		fmt.Fprintln(os.Stderr, "usage: alerts injury check [--dry-run] [--publish] [--force] [--team MIA] [--season 2026] [--week 1]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("injury check", flag.ExitOnError)
	var dryRun bool
	var publish bool
	var force bool
	var useTestChannel bool
	var team string
	var season int
	var week int
	fs.BoolVar(&dryRun, "dry-run", false, "Fetch, diff and render without publishing or writing state")
	fs.BoolVar(&publish, "publish", false, "Publish rendered messages and persist state")
	fs.BoolVar(&force, "force", false, "Bypass scraper cache and publication fingerprint checks")
	fs.BoolVar(&useTestChannel, "use-test-channel", false, "Use TELEGRAM_CHANNEL_ID_TEST when publishing")
	fs.StringVar(&team, "team", "MIA", "NFL team code")
	fs.IntVar(&season, "season", time.Now().Year(), "NFL season")
	fs.IntVar(&week, "week", 1, "NFL week")
	if err := fs.Parse(args[1:]); err != nil {
		os.Exit(2)
	}
	_ = godotenv.Load()
	if !dryRun && !publish {
		dryRun = true
	}

	cfg := config.Load(useTestChannel)
	provider := injury.NewDefaultProvider()
	store := injury.NewJSONReportStore(cfg.Injury.CacheFile)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report, err := provider.Fetch(ctx, injury.ReportRequest{
		Team:         injury.TeamCode(strings.ToUpper(team)),
		Season:       season,
		SeasonType:   injury.SeasonTypeREG,
		Week:         week,
		ForceRefresh: force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "injury check failed: %v\n", err)
		os.Exit(1)
	}
	previous, _ := store.LastReport(report.SnapshotKey())
	changes := injury.ReportDiffer{}.Diff(previous, report)
	fmt.Println(injury.TelegramRenderer{}.Render(report, changes))
	if dryRun {
		return
	}

	monitor := injury.NewMonitor(cfg.Injury.CacheFile, cfg.Telegram.BotToken, cfg.Telegram.SelectedChatID)
	monitor.SetProvider(provider)
	monitor.SetStore(store)
	_, err = monitor.Check(ctx, force, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "injury publish failed: %v\n", err)
		os.Exit(1)
	}
}
