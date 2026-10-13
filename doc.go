// Package dolphinsalerts is the module root of dolphins-alerts, the building
// blocks of the Mundo Dolphins alert services.
//
// Packages:
//
//   - espn: game alerts from ESPN's public scoreboard to Telegram
//   - injury: injury report model, diff, rendering and monitor; ships an
//     nflverse-backed provider and the InjuryProvider extension point
//   - socialfeed, rss: read an RSS feed of one account
//   - bluesky: publish posts to Bluesky
//   - telegram: Telegram Bot API sender
//   - config: environment configuration
//   - app: runs the monitors
//
// The module does not scrape third-party websites. Until v1.0.0 the API may
// change between minor versions.
package dolphinsalerts
