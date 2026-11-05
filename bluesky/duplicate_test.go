package bluesky

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	util "github.com/bluesky-social/indigo/lex/util"
)

// TestMain keeps tests off the network: by default the account has no posts.
func TestMain(m *testing.M) {
	feedGetAuthorFeed = feedWith()
	os.Exit(m.Run())
}

func TestSimilarText(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "episode with mentions and urls rewritten",
			a:    "🆕 Episodio ...\nEl tercer cuarto, el de los horrores, nos está lastrando esta temporada como vimos VS #Patriots\nPrevia 🆚 Lionsn con @PichuTeijeiro de @rugidos_detroit\n🎧 En iVoox, Spotify y https://www.mundodolphins.com\nEnlace ⤵️\nhttps://go.ivoox.com/rf/183306282",
			b:    "🆕 Episodio ...\nEl tercer cuarto, el de los horrores, nos está lastrando esta temporada como vimos frente a Patriots\nPrevia 🆚 Lions con Pichu Teijeiro de rugidos detroit \n🎧 En iVoox, Spotify y www.mundodolphins.com \nEnlace ⤵️\ngo.ivoox.com/rf/183306282",
			want: true,
		},
		{
			name: "whitespace and hashtag differences",
			a:    "IN MEMORIAN\nBob Heinz , DL\nIntegrante de \"No Name Defense\"\nCampeón 🏆x2 (72 y 73 )\nMiembro de @LosDolphins de 1969 a 1977.\nDescansa en Paz\n#PhinsUp",
			b:    "IN MEMORIAN \nBob Heinz , DL \nIntegrante de \"No Name Defense\"\nCampeón 🏆x2  (72 y 73 )\nMiembro de #LosDolphins de 1969 a 1977.\nDescansa en Paz \n#PhinsUp",
			want: true,
		},
		{
			name: "different posts",
			a:    "Los #Dolphins descansan hoy domingo pero Nosotros No!!",
			b:    "Nos invade la @jungla_es ! Recibimos a varios miembros para hablar de la temporada",
			want: false,
		},
		{
			name: "short texts must match exactly",
			a:    "Final del partido",
			b:    "Final del partido 2",
			want: false,
		},
		{
			name: "short identical texts",
			a:    "Final del partido!",
			b:    "final del  partido",
			want: true,
		},
		{name: "empty", a: "", b: "", want: false},
	}
	for _, tc := range cases {
		if got := similarText(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: similarText = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func feedWith(texts ...string) func(context.Context, util.LexClient, string, string, string, bool, int64) (*appbsky.FeedGetAuthorFeed_Output, error) {
	return func(context.Context, util.LexClient, string, string, string, bool, int64) (*appbsky.FeedGetAuthorFeed_Output, error) {
		out := &appbsky.FeedGetAuthorFeed_Output{}
		for _, text := range texts {
			out.Feed = append(out.Feed, &appbsky.FeedDefs_FeedViewPost{Post: &appbsky.FeedDefs_PostView{
				Record: &util.LexiconTypeDecoder{Val: &appbsky.FeedPost{Text: text, CreatedAt: time.Now().UTC().Format(time.RFC3339)}},
			}})
		}
		return out, nil
	}
}

func stubPosting(t *testing.T) (created *int) {
	t.Helper()
	oldPosted := postedHashesFile
	postedHashesFile = filepath.Join(t.TempDir(), "hashes.txt")
	oldSession, oldCreate := serverCreateSession, repoCreateRecord
	oldFeed := feedGetAuthorFeed
	t.Cleanup(func() {
		postedHashesFile = oldPosted
		serverCreateSession, repoCreateRecord = oldSession, oldCreate
		feedGetAuthorFeed = oldFeed
	})
	serverCreateSession = func(context.Context, util.LexClient, *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a", RefreshJwt: "r", Handle: "h", Did: "did:example:1"}, nil
	}
	n := 0
	repoCreateRecord = func(context.Context, util.LexClient, *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		n++
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	return &n
}

func TestPostToBluesky_SkipsPostAlreadyOnAccount(t *testing.T) {
	created := stubPosting(t)
	feedGetAuthorFeed = feedWith("IN MEMORIAN \nBob Heinz , DL \nDescansa en Paz \n#PhinsUp")

	if err := PostToBluesky(context.Background(), "IN MEMORIAN\nBob Heinz , DL\nDescansa en Paz\n#PhinsUp", nil, nil); err != nil {
		t.Fatal(err)
	}
	if *created != 0 {
		t.Fatalf("duplicate was published (%d records)", *created)
	}
}

func TestPostToBluesky_PublishesWhenNoDuplicate(t *testing.T) {
	created := stubPosting(t)
	feedGetAuthorFeed = feedWith("Algo totalmente distinto sobre otro tema de la semana")

	if err := PostToBluesky(context.Background(), "IN MEMORIAN\nBob Heinz , DL\nDescansa en Paz\n#PhinsUp", nil, nil); err != nil {
		t.Fatal(err)
	}
	if *created != 1 {
		t.Fatalf("created = %d, want 1", *created)
	}
}

func TestPostToBluesky_PublishesWhenFeedLookupFails(t *testing.T) {
	created := stubPosting(t)
	feedGetAuthorFeed = func(context.Context, util.LexClient, string, string, string, bool, int64) (*appbsky.FeedGetAuthorFeed_Output, error) {
		return nil, errors.New("boom")
	}
	if err := PostToBluesky(context.Background(), "Hola mundo delfín", nil, nil); err != nil {
		t.Fatal(err)
	}
	if *created != 1 {
		t.Fatalf("created = %d, want 1", *created)
	}
}

func TestSavePostedHashCreatesDirectory(t *testing.T) {
	old := postedHashesFile
	postedHashesFile = filepath.Join(t.TempDir(), "nested", "hashes.txt")
	defer func() { postedHashesFile = old }()
	savePostedHash("abc")
	if _, err := os.Stat(postedHashesFile); err != nil {
		t.Fatalf("hash file not written: %v", err)
	}
}
