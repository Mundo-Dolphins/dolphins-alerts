package bluesky

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	util "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/rivo/uniseg"
)

// Basic configuration — credentials should be provided via environment variables
var (
	BlueskyHandle    string // Se inicializa en main.go desde .env
	BlueskyAppPass   string // Se inicializa en main.go desde .env
	postedHashesFile = "data/bluesky_posted_hashes.txt"
)

// mediaClient downloads post attachments; it has a timeout so a stalled host
// cannot block publishing forever.
var mediaClient = &http.Client{Timeout: 30 * time.Second}

// maxImages is the number of images Bluesky allows in a single post.
const maxImages = 4

// injection points for testing
var (
	httpGet             = mediaClient.Get
	serverCreateSession = atproto.ServerCreateSession
	repoUploadBlob      = atproto.RepoUploadBlob
	repoCreateRecord    = atproto.RepoCreateRecord
)

var urlPattern = regexp.MustCompile(`https?://[^\s]+`)

// urlSpans returns the byte ranges of the URLs in text, without the
// punctuation that usually trails a link in prose.
func urlSpans(text string) [][2]int {
	var spans [][2]int
	for _, m := range urlPattern.FindAllStringIndex(text, -1) {
		end := m[0] + len(strings.TrimRight(text[m[0]:m[1]], `.,;:!?)]}'"…`))
		if end > m[0] {
			spans = append(spans, [2]int{m[0], end})
		}
	}
	return spans
}

// linkFacets marks every URL in text as a clickable link; without facets
// Bluesky renders URLs as plain text.
func linkFacets(text string) []*appbsky.RichtextFacet {
	var facets []*appbsky.RichtextFacet
	for _, span := range urlSpans(text) {
		facets = append(facets, &appbsky.RichtextFacet{
			Index: &appbsky.RichtextFacet_ByteSlice{ByteStart: int64(span[0]), ByteEnd: int64(span[1])},
			Features: []*appbsky.RichtextFacet_Features_Elem{
				{RichtextFacet_Link: &appbsky.RichtextFacet_Link{Uri: text[span[0]:span[1]]}},
			},
		})
	}
	return facets
}

// truncateText recorta el texto a un máximo de graphemes especificado. Nunca
// corta una URL por la mitad: si el recorte cae dentro de una, se descarta entera.
func truncateText(text string, maxGraphemes int) string {
	if uniseg.GraphemeClusterCount(text) <= maxGraphemes {
		return text
	}

	var result strings.Builder
	gr := uniseg.NewGraphemes(text)
	count := 0

	for gr.Next() {
		if count >= maxGraphemes-2 { // Reservar espacio para "..."
			break
		}
		result.Write(gr.Bytes())
		count++
	}

	cut := result.Len()
	for _, span := range urlSpans(text) {
		if span[0] < cut && cut < span[1] {
			cut = span[0]
			break
		}
	}
	return strings.TrimRight(text[:cut], " \n") + "..."
}

func getTweetHash(text string, images, videos []string) string {
	h := sha256.New()
	h.Write([]byte(text))
	for _, img := range images {
		h.Write([]byte(img))
	}
	for _, vid := range videos {
		h.Write([]byte(vid))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func isHashPosted(hash string) bool {
	b, err := os.ReadFile(postedHashesFile)
	if err != nil {
		return false
	}
	lines := strings.Split(string(b), "\n")
	for _, l := range lines {
		if l == hash {
			return true
		}
	}
	return false
}

func savePostedHash(hash string) {
	f, err := os.OpenFile(postedHashesFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(hash + "\n")
		_ = f.Close()
	}
}

func PostToBluesky(ctx context.Context, text string, images, videos []string) error {
	// Truncate text to a maximum number of grapheme clusters for Bluesky
	truncatedText := truncateText(text, 300)
	if len(truncatedText) != len(text) {
		log.Printf("Text truncated from %d to %d graphemes", uniseg.GraphemeClusterCount(text), uniseg.GraphemeClusterCount(truncatedText))
	}

	fullText := truncatedText
	hash := getTweetHash(fullText, images, videos)
	if isHashPosted(hash) {
		log.Printf("Post already published on Bluesky (hash: %s)", hash)
		return nil
	}
	client := &xrpc.Client{Host: "https://bsky.social"}
	// Autenticación
	sess, err := serverCreateSession(ctx, client, &atproto.ServerCreateSession_Input{
		Identifier: BlueskyHandle,
		Password:   BlueskyAppPass,
	})
	if err != nil {
		return err
	}
	client.Auth = &xrpc.AuthInfo{
		AccessJwt:  sess.AccessJwt,
		RefreshJwt: sess.RefreshJwt,
		Handle:     sess.Handle,
		Did:        sess.Did,
	}
	now := time.Now().UTC().Format(time.RFC3339)

	var embed *appbsky.FeedPost_Embed
	// Bluesky allows a single video per post and it cannot be combined with
	// images. If the video cannot be mirrored, fall back to the images.
	if len(videos) > 0 {
		log.Printf("Uploading video: %s", videos[0])
		if blob, err := uploadVideo(ctx, client, sess.Did, videos[0]); err != nil {
			log.Printf("Skipping video %s: %v", videos[0], err)
		} else {
			alt := "Attached video"
			embed = &appbsky.FeedPost_Embed{EmbedVideo: &appbsky.EmbedVideo{Video: blob, Alt: &alt}}
		}
	}
	if embed == nil {
		if len(images) > maxImages {
			images = images[:maxImages]
		}
		var blobRefs []*appbsky.EmbedImages_Image
		for _, imgURL := range images {
			blob, err := uploadImage(ctx, client, imgURL)
			if err != nil {
				log.Printf("Skipping image %s: %v", imgURL, err)
				continue
			}
			blobRefs = append(blobRefs, &appbsky.EmbedImages_Image{Image: blob, Alt: "Attached image"})
		}
		if len(blobRefs) > 0 {
			embed = &appbsky.FeedPost_Embed{EmbedImages: &appbsky.EmbedImages{Images: blobRefs}}
		}
	}
	record := &appbsky.FeedPost{
		Text:      fullText,
		Facets:    linkFacets(fullText),
		CreatedAt: now,
		Embed:     embed,
	}
	lexRecord := &util.LexiconTypeDecoder{Val: record}
	_, err = repoCreateRecord(ctx, client, &atproto.RepoCreateRecord_Input{
		Repo:       sess.Did,
		Collection: "app.bsky.feed.post",
		Record:     lexRecord,
	})
	if err == nil {
		savePostedHash(hash)
	} else {
		log.Printf("Error publishing to Bluesky: %v", err)
	}
	return err
}
