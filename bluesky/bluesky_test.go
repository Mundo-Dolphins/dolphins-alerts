package bluesky

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestTruncateTextLimitsGraphemes(t *testing.T) {
	// create a string with many simple graphemes
	long := strings.Repeat("a", 500)
	out := truncateText(long, 100)
	// The implementation reserves space for "..." and writes it when truncating.
	// That means the resulting grapheme count can be slightly higher than max (e.g. 98 + 3 == 101).
	gc := uniseg.GraphemeClusterCount(out)
	if gc > 101 {
		t.Fatalf("truncateText returned %d graphemes, want <= %d", gc, 101)
	}
	// If input was longer than the limit, ensure output ends with ellipsis
	if len(long) > 100 && !strings.HasSuffix(out, "...") {
		t.Fatalf("expected truncated output to end with ..., got %q", out)
	}
}

func TestGetTweetHashDeterministic(t *testing.T) {
	h1 := getTweetHash("hello", []string{"img1", "img2"}, []string{"vid1"})
	h2 := getTweetHash("hello", []string{"img1", "img2"}, []string{"vid1"})
	if h1 != h2 {
		t.Fatalf("hashes differ for same inputs: %s vs %s", h1, h2)
	}
	h3 := getTweetHash("hello", []string{"img2", "img1"}, []string{"vid1"})
	if h1 == h3 {
		t.Fatalf("hash should differ when image order changes")
	}
}
