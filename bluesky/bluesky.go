package bluesky

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
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

// maxImageBytes bounds an attachment (Bluesky itself rejects images over ~1 MB).
const maxImageBytes = 2 << 20

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

	// Descargar y subir blobs
	var blobRefs []*appbsky.EmbedImages_Image
	if len(images) > maxImages {
		images = images[:maxImages]
	}
	for _, imgURL := range images {
		log.Printf("Downloading image: %s", imgURL)
		resp, err := httpGet(imgURL)
		if err != nil {
			log.Printf("Error downloading image: %s", imgURL)
			continue
		}
		mimeType := resp.Header.Get("Content-Type")
		log.Printf("Detected MIME type: %s", mimeType)
		// Bluesky rejects the whole post if an embedded blob is not an image.
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(mimeType, "image/") {
			_ = resp.Body.Close()
			log.Printf("Skipping image %s: status=%d MIME=%s", imgURL, resp.StatusCode, mimeType)
			continue
		}
		blobResp, err := repoUploadBlob(ctx, client, io.LimitReader(resp.Body, maxImageBytes))
		_ = resp.Body.Close()
		if err != nil {
			log.Printf("Error uploading blob: %s | MIME: %s | Error: %v", imgURL, mimeType, err)
			continue
		}
		log.Printf("Blob uploaded successfully: Ref=%s, MIME=%s", blobResp.Blob.Ref, mimeType)
		blobRefs = append(blobRefs, &appbsky.EmbedImages_Image{
			Image: blobResp.Blob,
			Alt:   "Attached image",
		})
	}
	// Bluesky posts only embed images, so videos are ignored: they are not
	// downloaded.
	_ = videos

	var embed *appbsky.FeedPost_Embed
	if len(blobRefs) > 0 {
		embed = &appbsky.FeedPost_Embed{
			EmbedImages: &appbsky.EmbedImages{
				Images: blobRefs,
			},
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
