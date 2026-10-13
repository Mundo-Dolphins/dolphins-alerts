package config

import (
	"os"
	"strings"
	"time"
)

const (
	DefaultESPNCacheFile     = "espn_cache.json"
	DefaultInjuryCacheFile   = "injury_cache.json"
	DefaultSocialFeedBaseURL = "http://127.0.0.1:8090"
)

type Config struct {
	Telegram TelegramConfig
	Bluesky  BlueskyConfig
	Feed     SocialFeedConfig
	Injury   InjuryConfig
	ESPN     ESPNConfig
}

type TelegramConfig struct {
	BotToken       string
	ChannelID      string
	TestChannelID  string
	SelectedChatID string
	UseTestChannel bool
}

type BlueskyConfig struct {
	Handle      string
	AppPassword string
}

// SocialFeedConfig describes the RSS feed of the account that is mirrored to
// Bluesky: the posts are read from {BaseURL}/user/{Target}.
type SocialFeedConfig struct {
	Target       string
	BaseURL      string
	PollInterval time.Duration
	SeenFile     string
}

type InjuryConfig struct {
	CacheFile string
}

type ESPNConfig struct {
	CacheFile string
}

func Load(useTestChannel bool) Config {
	telegram := TelegramConfig{
		BotToken:       os.Getenv("TELEGRAM_BOT_TOKEN"),
		ChannelID:      os.Getenv("TELEGRAM_CHANNEL_ID"),
		TestChannelID:  os.Getenv("TELEGRAM_CHANNEL_ID_TEST"),
		UseTestChannel: useTestChannel,
	}
	if useTestChannel {
		telegram.SelectedChatID = telegram.TestChannelID
	} else {
		telegram.SelectedChatID = telegram.ChannelID
	}

	return Config{
		Telegram: telegram,
		Bluesky: BlueskyConfig{
			Handle:      os.Getenv("BLUESKY_HANDLE"),
			AppPassword: os.Getenv("BLUESKY_APP_PASSWORD"),
		},
		Feed: SocialFeedConfig{
			Target:       strings.TrimPrefix(firstEnv("SOCIAL_FEED_TARGET", "TWITTER_TARGET"), "@"),
			BaseURL:      strings.TrimRight(envOrDefault2(DefaultSocialFeedBaseURL, "SOCIAL_FEED_BASE_URL", "TWITTER_RSS_BASE_URL"), "/"),
			PollInterval: 5 * time.Minute,
			SeenFile:     "last_twitter_id.txt",
		},
		Injury: InjuryConfig{
			CacheFile: DefaultInjuryCacheFile,
		},
		ESPN: ESPNConfig{
			CacheFile: envOrDefault("ESPN_CACHE_FILE", DefaultESPNCacheFile),
		},
	}
}

// firstEnv returns the first non-empty variable among keys. The TWITTER_*
// names are kept as aliases for existing deployments.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func envOrDefault2(fallback string, keys ...string) string {
	if v := firstEnv(keys...); v != "" {
		return v
	}
	return fallback
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
