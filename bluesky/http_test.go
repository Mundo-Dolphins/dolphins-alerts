package bluesky

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	util "github.com/bluesky-social/indigo/lex/util"
)

func TestPostToBluesky_DownloadAndUploadMocks(t *testing.T) {
	// prepare temp posted hashes file
	tmp, err := os.CreateTemp("", "posted_hashes-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = tmp.Close()
	oldPosted := postedHashesFile
	postedHashesFile = tmp.Name()
	defer func() {
		postedHashesFile = oldPosted
		_ = os.Remove(tmp.Name())
	}()

	// mock serverCreateSession
	oldSession := serverCreateSession
	serverCreateSession = func(ctx context.Context, c util.LexClient, input *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{
			AccessJwt:  "access",
			RefreshJwt: "refresh",
			Handle:     "h",
			Did:        "did:example:1",
		}, nil
	}
	defer func() { serverCreateSession = oldSession }()

	// mock httpGet to return an image body
	oldGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		body := io.NopCloser(bytes.NewReader([]byte("imagedata")))
		return &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": []string{"image/png"}}}, nil
	}
	defer func() { httpGet = oldGet }()

	// mock repoUploadBlob to read the passed reader and return a fake blob
	oldUpload := repoUploadBlob
	repoUploadBlob = func(ctx context.Context, c util.LexClient, input io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		// read the input to ensure it's forwarded
		b, _ := io.ReadAll(input)
		if !bytes.Equal(b, []byte("imagedata")) {
			t.Fatalf("unexpected upload body: %q", string(b))
		}
		return &atproto.RepoUploadBlob_Output{Blob: &util.LexBlob{MimeType: "image/png", Size: int64(len(b))}}, nil
	}
	defer func() { repoUploadBlob = oldUpload }()

	// mock repoCreateRecord to just succeed
	oldCreate := repoCreateRecord
	repoCreateRecord = func(ctx context.Context, c util.LexClient, input *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	defer func() { repoCreateRecord = oldCreate }()

	// Call PostToBluesky
	err = PostToBluesky(context.Background(), "Hello world", []string{"http://example/image.png"}, nil)
	if err != nil {
		t.Fatalf("PostToBluesky returned error: %v", err)
	}

	// ensure postedHashesFile now contains an entry
	info, err := os.ReadFile(postedHashesFile)
	if err != nil {
		t.Fatalf("failed to read posted hashes: %v", err)
	}
	if len(info) == 0 {
		t.Fatalf("expected posted hashes to be saved")
	}

	// Run again to ensure duplicate post is skipped (isHashPosted path)
	// give small time gap
	time.Sleep(5 * time.Millisecond)
	if err := PostToBluesky(context.Background(), "Hello world", []string{"http://example/image.png"}, nil); err != nil {
		t.Fatalf("second PostToBluesky should not fail: %v", err)
	}
}

func TestPostToBluesky_DoesNotDownloadVideosAndLimitsImages(t *testing.T) {
	tmp := t.TempDir() + "/hashes.txt"
	oldPosted := postedHashesFile
	postedHashesFile = tmp
	defer func() { postedHashesFile = oldPosted }()

	oldSession := serverCreateSession
	serverCreateSession = func(context.Context, util.LexClient, *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a", RefreshJwt: "r", Handle: "h", Did: "did:example:1"}, nil
	}
	defer func() { serverCreateSession = oldSession }()

	oldCreate := repoCreateRecord
	repoCreateRecord = func(context.Context, util.LexClient, *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	defer func() { repoCreateRecord = oldCreate }()

	// An image larger than maxImageBytes must be truncated, not forwarded whole.
	oldGet := httpGet
	httpGet = func(string) (*http.Response, error) {
		body := io.NopCloser(bytes.NewReader(make([]byte, maxImageBytes+1024)))
		return &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": []string{"image/png"}}}, nil
	}
	defer func() { httpGet = oldGet }()
	var uploaded int
	oldUpload := repoUploadBlob
	repoUploadBlob = func(_ context.Context, _ util.LexClient, input io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		b, _ := io.ReadAll(input)
		uploaded = len(b)
		return &atproto.RepoUploadBlob_Output{Blob: &util.LexBlob{MimeType: "image/png", Size: int64(len(b))}}, nil
	}
	defer func() { repoUploadBlob = oldUpload }()

	// A video URL must never be requested.
	videoHits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { videoHits++ }))
	defer ts.Close()

	if err := PostToBluesky(context.Background(), "big image", []string{"http://example/big.png"}, []string{ts.URL + "/video.mp4"}); err != nil {
		t.Fatal(err)
	}
	if uploaded != maxImageBytes {
		t.Fatalf("uploaded %d bytes, want it capped at %d", uploaded, maxImageBytes)
	}
	if videoHits != 0 {
		t.Fatalf("video URL was requested %d times", videoHits)
	}
}
