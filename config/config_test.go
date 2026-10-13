package config

import "testing"

func TestLoadDefaultsAndSelectedTelegramChannel(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHANNEL_ID", "@prod")
	t.Setenv("TELEGRAM_CHANNEL_ID_TEST", "@test")
	t.Setenv("BLUESKY_HANDLE", "handle")
	t.Setenv("BLUESKY_APP_PASSWORD", "pass")

	cfg := Load(true)

	if cfg.Telegram.SelectedChatID != "@test" {
		t.Fatalf("expected test channel, got %q", cfg.Telegram.SelectedChatID)
	}
	if cfg.Feed.BaseURL != DefaultSocialFeedBaseURL {
		t.Fatalf("expected default feed base URL, got %q", cfg.Feed.BaseURL)
	}
	if cfg.Injury.CacheFile != DefaultInjuryCacheFile {
		t.Fatalf("expected default injury cache file, got %q", cfg.Injury.CacheFile)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("TELEGRAM_CHANNEL_ID", "@prod")
	t.Setenv("SOCIAL_FEED_TARGET", "@miamidolphins")
	t.Setenv("SOCIAL_FEED_BASE_URL", "http://rss.local/")
	t.Setenv("ESPN_CACHE_FILE", "cache/espn.json")

	cfg := Load(false)

	if cfg.Telegram.SelectedChatID != "@prod" {
		t.Fatalf("expected prod channel, got %q", cfg.Telegram.SelectedChatID)
	}
	if cfg.Feed.Target != "miamidolphins" {
		t.Fatalf("expected trimmed feed target, got %q", cfg.Feed.Target)
	}
	if cfg.Feed.BaseURL != "http://rss.local" {
		t.Fatalf("expected trimmed feed base URL, got %q", cfg.Feed.BaseURL)
	}
	if cfg.ESPN.CacheFile != "cache/espn.json" {
		t.Fatalf("expected ESPN cache override, got %q", cfg.ESPN.CacheFile)
	}
}

// Existing deployments use the TWITTER_* names; they must keep working.
func TestLoadLegacyTwitterAliases(t *testing.T) {
	t.Setenv("TWITTER_TARGET", "@miamidolphins")
	t.Setenv("TWITTER_RSS_BASE_URL", "http://legacy.local/")

	cfg := Load(false)

	if cfg.Feed.Target != "miamidolphins" || cfg.Feed.BaseURL != "http://legacy.local" {
		t.Fatalf("legacy aliases not honoured: %+v", cfg.Feed)
	}
}

func TestLoadNewNamesWinOverLegacyAliases(t *testing.T) {
	t.Setenv("SOCIAL_FEED_TARGET", "new")
	t.Setenv("TWITTER_TARGET", "old")

	if got := Load(false).Feed.Target; got != "new" {
		t.Fatalf("expected SOCIAL_FEED_TARGET to win, got %q", got)
	}
}
