package bluesky

import (
	"context"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	util "github.com/bluesky-social/indigo/lex/util"
)

func linkFacetURIs(t *testing.T, text string) (uris []string, slices []string) {
	t.Helper()
	for _, f := range linkFacets(text) {
		if len(f.Features) != 1 || f.Features[0].RichtextFacet_Link == nil {
			t.Fatalf("expected a single link feature, got %+v", f.Features)
		}
		uris = append(uris, f.Features[0].RichtextFacet_Link.Uri)
		slices = append(slices, text[f.Index.ByteStart:f.Index.ByteEnd])
	}
	return uris, slices
}

func TestLinkFacets_ByteOffsetsWithEmoji(t *testing.T) {
	text := "🆕 Episodio ⤵️ https://mundodolphins.es/podcast/previa-jets/ fin"
	uris, slices := linkFacetURIs(t, text)
	want := "https://mundodolphins.es/podcast/previa-jets/"
	if len(uris) != 1 || uris[0] != want || slices[0] != want {
		t.Fatalf("uris=%v slices=%v, want %q", uris, slices, want)
	}
}

func TestLinkFacets_TrimsTrailingPunctuation(t *testing.T) {
	uris, slices := linkFacetURIs(t, "Mira (https://example.com/a), y https://example.com/b.")
	if len(uris) != 2 || uris[0] != "https://example.com/a" || uris[1] != "https://example.com/b" {
		t.Fatalf("uris=%v slices=%v", uris, slices)
	}
}

func TestLinkFacets_NoURLs(t *testing.T) {
	if got := linkFacets("sin enlaces #Dolphins @user"); len(got) != 0 {
		t.Fatalf("expected no facets, got %+v", got)
	}
}

func TestTruncateText_NeverCutsInsideURL(t *testing.T) {
	url := "https://mundodolphins.es/podcast/" + strings.Repeat("x", 60)
	text := strings.Repeat("a", 90) + " " + url
	out := truncateText(text, 100)
	if strings.Contains(out, "https://") {
		t.Fatalf("a URL cut in the middle must be dropped entirely, got %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("expected ellipsis, got %q", out)
	}
	if got := truncateText(text, 400); got != text {
		t.Fatalf("text under the limit must be untouched")
	}
}

func TestPostToBluesky_RecordCarriesLinkFacets(t *testing.T) {
	oldPosted := postedHashesFile
	postedHashesFile = t.TempDir() + "/hashes.txt"
	defer func() { postedHashesFile = oldPosted }()

	oldSession := serverCreateSession
	serverCreateSession = func(context.Context, util.LexClient, *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a", RefreshJwt: "r", Handle: "h", Did: "did:example:1"}, nil
	}
	defer func() { serverCreateSession = oldSession }()

	var got *appbsky.FeedPost
	oldCreate := repoCreateRecord
	repoCreateRecord = func(_ context.Context, _ util.LexClient, in *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		got, _ = in.Record.Val.(*appbsky.FeedPost)
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	defer func() { repoCreateRecord = oldCreate }()

	if err := PostToBluesky(context.Background(), "Escucha https://mundodolphins.es/podcast/x/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Facets) != 1 {
		t.Fatalf("expected one link facet in the record, got %+v", got)
	}
	if uri := got.Facets[0].Features[0].RichtextFacet_Link.Uri; uri != "https://mundodolphins.es/podcast/x/" {
		t.Fatalf("facet uri = %q", uri)
	}
}
