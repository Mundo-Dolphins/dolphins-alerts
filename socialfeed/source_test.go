package socialfeed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mundo-Dolphins/dolphins-alerts/rss"
)

func TestSourceFetchNormal(t *testing.T) {
	source, requests := sourceFromFixture(t, "normal.xml")

	posts, err := source.Fetch(context.Background(), "@exampleteam")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	if got := *requests; got != "/user/exampleteam" {
		t.Fatalf("expected safe proxy path, got %q", got)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	if posts[0].ID != "2097328956898693527" {
		t.Fatalf("expected extracted status ID, got %q", posts[0].ID)
	}
	if posts[0].Text != "First & ready #PhinsUp https://t.co/ABC123" {
		t.Fatalf("unexpected cleaned text: %q", posts[0].Text)
	}
	if posts[1].Text != "Hello @someplayer & fans" {
		t.Fatalf("unexpected mention text: %q", posts[1].Text)
	}
	if posts[0].Platform != PlatformTwitterX || posts[0].Account != "exampleteam" {
		t.Fatalf("unexpected post metadata: %+v", posts[0])
	}
}

func TestSourceFetchMultilineAndEmoji(t *testing.T) {
	source, _ := sourceFromFixture(t, "multiline.xml")

	posts, err := source.Fetch(context.Background(), "exampleteam")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	expected := "South Florida, it’s time.\n\nWeek 1 is here 🐬"
	if posts[0].Text != expected {
		t.Fatalf("expected multiline text %q, got %q", expected, posts[0].Text)
	}

	source, _ = sourceFromFixture(t, "emoji.xml")
	posts, err = source.Fetch(context.Background(), "exampleteam")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if posts[0].Text != "🐬 🫡 🎉 🙂‍↕️" {
		t.Fatalf("expected emoji preservation, got %q", posts[0].Text)
	}
}

func TestSourceFetchSoloEmojiURLKeepsEmojiAndURL(t *testing.T) {
	source, _ := sourceFromFixture(t, "solo_emoji_url.xml")

	posts, err := source.Fetch(context.Background(), "exampleteam")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	if posts[0].Text != "🙂‍↕️ https://t.co/ONLYURL" {
		t.Fatalf("expected emoji and URL to be preserved, got %q", posts[0].Text)
	}
}

func TestSourceFetchKeepsLinksAndImages(t *testing.T) {
	source, _ := sourceFromFixture(t, "links_media.xml")

	posts, err := source.Fetch(context.Background(), "mundodolphins")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	want := "🆕 Episodio\nEnlace ⤵️\nhttps://mundodolphins.es/podcast/previa-jets/"
	if posts[0].Text != want {
		t.Fatalf("text = %q, want %q", posts[0].Text, want)
	}
	wantImages := []string{"https://pbs.twimg.com/media/AAA.jpg", "https://pbs.twimg.com/media/BBB.png"}
	if !reflect.DeepEqual(posts[0].Images, wantImages) {
		t.Fatalf("images = %v, want %v", posts[0].Images, wantImages)
	}
	wantVideos := []string{"https://video.twimg.com/ext_tw_video/1.mp4"}
	if !reflect.DeepEqual(posts[0].Videos, wantVideos) {
		t.Fatalf("videos = %v, want %v", posts[0].Videos, wantVideos)
	}
	if len(posts[1].Images) != 0 {
		t.Fatalf("expected no images, got %v", posts[1].Images)
	}
}

func TestNormalizeHandleRejectsURLInjection(t *testing.T) {
	normalized, err := NormalizeHandle("@ExampleTeam")
	if err != nil {
		t.Fatalf("NormalizeHandle returned error: %v", err)
	}
	if normalized != "exampleteam" {
		t.Fatalf("expected lowercase normalized handle, got %q", normalized)
	}

	bad := []string{"", "@", "../miami", "miami/rss", "miami?x=1", "http://x"}
	for _, handle := range bad {
		if _, err := NormalizeHandle(handle); err == nil {
			t.Fatalf("expected invalid handle for %q", handle)
		}
	}
}

func TestNewPostsOrderAndIdempotence(t *testing.T) {
	posts := []SocialPost{
		{ID: "E"}, {ID: "D"}, {ID: "A"}, {ID: "B"}, {ID: "C"},
	}
	seen := &memorySeen{ids: map[string]struct{}{"A": {}, "B": {}, "C": {}}}

	fresh := NewPosts(posts, seen)
	got := ids(fresh)
	expected := []string{"D", "E"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}

	for _, post := range fresh {
		seen.ids[post.ID] = struct{}{}
	}
	if got := NewPosts(posts, seen); len(got) != 0 {
		t.Fatalf("expected idempotent empty result, got %+v", got)
	}
}

func TestSeenStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last_twitter_id.txt")
	store := NewSeenStore(path)
	store.Mark("B")
	store.Mark("A")
	if err := store.Save(); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded := NewSeenStore(path)
	if err := loaded.Load(); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !loaded.IsSeen("A") || !loaded.IsSeen("B") {
		t.Fatalf("expected IDs to round trip")
	}
}

func TestCorruptXMLDoesNotModifySeenState(t *testing.T) {
	source, _ := sourceFromFixture(t, "corrupt.xml")
	store := NewSeenStore(filepath.Join(t.TempDir(), "seen.txt"))
	store.Mark("existing")

	_, err := source.Fetch(context.Background(), "exampleteam")
	if !errors.Is(err, rss.ErrInvalidXML) {
		t.Fatalf("expected invalid XML error, got %v", err)
	}

	if !store.IsSeen("existing") || store.IsSeen("broken") {
		t.Fatalf("seen state changed after corrupt XML")
	}
}

func TestEmptyFeedIsNotError(t *testing.T) {
	source := sourceFromBody(t, `<rss><channel><title>Empty</title></channel></rss>`, nil)

	posts, err := source.Fetch(context.Background(), "exampleteam")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("expected empty posts, got %+v", posts)
	}
}

func TestInvalidItemReturnsError(t *testing.T) {
	source := sourceFromBody(t, `<rss><channel><item><description>Missing identifiers</description></item></channel></rss>`, nil)

	_, err := source.Fetch(context.Background(), "exampleteam")
	if !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("expected invalid item error, got %v", err)
	}
}

func TestHTTPAndDateErrors(t *testing.T) {
	statusSource := sourceFromHandler(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	})
	if _, err := statusSource.Fetch(context.Background(), "exampleteam"); !errors.Is(err, rss.ErrHTTPStatus) {
		t.Fatalf("expected HTTP status error, got %v", err)
	}

	dateSource := sourceFromBody(t, `<rss><channel><item><guid>1</guid><pubDate>not a date</pubDate></item></channel></rss>`, nil)
	if _, err := dateSource.Fetch(context.Background(), "exampleteam"); !errors.Is(err, rss.ErrInvalidPubDate) {
		t.Fatalf("expected invalid pubDate error, got %v", err)
	}
}

func TestCleanTwitterTextDecodesAtMostTwice(t *testing.T) {
	if got := CleanTwitterText("&amp;"); got != "&" {
		t.Fatalf("expected ampersand, got %q", got)
	}
	if got := CleanTwitterText("&amp;amp;"); got != "&" {
		t.Fatalf("expected double encoded ampersand, got %q", got)
	}
	if got := CleanTwitterText("&amp;amp;amp;"); got != "&amp;" {
		t.Fatalf("expected bounded decoding, got %q", got)
	}
}

func sourceFromFixture(t *testing.T, name string) (*Source, *string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var path string
	return sourceFromBody(t, string(body), &path), &path
}

func sourceFromBody(t *testing.T, body string, requestPath *string) *Source {
	t.Helper()
	return sourceFromHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if requestPath != nil {
			*requestPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	})
}

func sourceFromHandler(t *testing.T, handler http.HandlerFunc) *Source {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewSource(server.URL, nil)
}

type memorySeen struct {
	ids map[string]struct{}
}

func (m *memorySeen) IsSeen(id string) bool {
	_, ok := m.ids[id]
	return ok
}

func ids(posts []SocialPost) []string {
	values := make([]string, 0, len(posts))
	for _, post := range posts {
		values = append(values, post.ID)
	}
	return values
}

func TestNewPostsFixtureScenario(t *testing.T) {
	source, _ := sourceFromFixture(t, "new_posts.xml")

	posts, err := source.Fetch(context.Background(), "exampleteam")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	seen := &memorySeen{ids: map[string]struct{}{"A": {}, "B": {}, "C": {}}}
	got := strings.Join(ids(NewPosts(posts, seen)), ",")
	if got != "D,E" {
		t.Fatalf("expected D,E, got %s", got)
	}
}
