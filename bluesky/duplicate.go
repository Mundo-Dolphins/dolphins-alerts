package bluesky

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

const (
	// duplicateLookback is how far back existing posts are compared against a
	// new one; the same content is usually published minutes apart.
	duplicateLookback = 24 * time.Hour
	// duplicateFeedLimit is how many recent posts of the account are fetched.
	duplicateFeedLimit = 50
	// similarityThreshold is the minimum Dice coefficient to call two texts the
	// same post. Other publishers rewrite @mentions, #hashtags, URLs and
	// spacing, so exact equality never matches.
	similarityThreshold = 0.8
	// minFuzzyTokens is the minimum number of words for fuzzy matching; shorter
	// texts must match exactly to avoid merging distinct short posts.
	minFuzzyTokens = 6
)

var feedGetAuthorFeed = appbsky.FeedGetAuthorFeed

var (
	schemeURLPattern = regexp.MustCompile(`(?i)(https?://|www\.)\S+`)
	bareURLPattern   = regexp.MustCompile(`(?i)\b[\p{L}\d-]+(\.[\p{L}\d-]+)+(/\S*)?`)
)

// textTokens lowercases text and returns its words, ignoring URLs and
// punctuation so cosmetic rewrites do not change the result.
func textTokens(text string) []string {
	text = schemeURLPattern.ReplaceAllString(text, " ")
	text = bareURLPattern.ReplaceAllString(text, " ")
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// similarText reports whether a and b are the same post modulo the cosmetic
// differences between publishers (mentions, hashtags, URLs, whitespace).
func similarText(a, b string) bool {
	ta, tb := textTokens(a), textTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	if strings.Join(ta, " ") == strings.Join(tb, " ") {
		return true
	}
	if len(ta) < minFuzzyTokens || len(tb) < minFuzzyTokens {
		return false
	}
	setA := make(map[string]struct{}, len(ta))
	for _, w := range ta {
		setA[w] = struct{}{}
	}
	setB := make(map[string]struct{}, len(tb))
	for _, w := range tb {
		setB[w] = struct{}{}
	}
	common := 0
	for w := range setA {
		if _, ok := setB[w]; ok {
			common++
		}
	}
	return 2*float64(common)/float64(len(setA)+len(setB)) >= similarityThreshold
}

// findDuplicate returns the URI of a recent post of the account whose text
// matches text, and whether there is one. Lookup errors are returned so the caller can decide to publish
// anyway.
func findDuplicate(ctx context.Context, client *xrpc.Client, did, text string) (string, bool, error) {
	out, err := feedGetAuthorFeed(ctx, client, did, "", "posts_no_replies", false, duplicateFeedLimit)
	if err != nil {
		return "", false, err
	}
	cutoff := time.Now().Add(-duplicateLookback)
	for _, item := range out.Feed {
		if item == nil || item.Post == nil || item.Reason != nil || item.Post.Record == nil {
			continue
		}
		rec, ok := item.Post.Record.Val.(*appbsky.FeedPost)
		if !ok {
			continue
		}
		if created, err := time.Parse(time.RFC3339, rec.CreatedAt); err == nil && created.Before(cutoff) {
			continue
		}
		if similarText(text, rec.Text) {
			return item.Post.Uri, true, nil
		}
	}
	return "", false, nil
}

func logDuplicateLookupError(err error) {
	log.Printf("Could not check Bluesky for duplicates, publishing anyway: %v", err)
}
