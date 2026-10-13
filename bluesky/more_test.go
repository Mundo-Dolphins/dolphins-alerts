package bluesky

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/bluesky-social/indigo/api/atproto"
	util "github.com/bluesky-social/indigo/lex/util"
)

func TestSaveAndIsHashPosted(t *testing.T) {
	tmp, err := os.CreateTemp("", "hashes-*.txt")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_ = tmp.Close()
	old := postedHashesFile
	postedHashesFile = tmp.Name()
	defer func() { postedHashesFile = old; _ = os.Remove(tmp.Name()) }()

	h := "abc123"
	savePostedHash(h)
	if !isHashPosted(h) {
		t.Fatalf("expected hash to be present")
	}
}

func TestPostToBluesky_UploadErrorStillPosts(t *testing.T) {
	// prepare temp posted hashes file
	tmp, _ := os.CreateTemp("", "posted-*.txt")
	_ = tmp.Close()
	oldPosted := postedHashesFile
	postedHashesFile = tmp.Name()
	defer func() { postedHashesFile = oldPosted; _ = os.Remove(tmp.Name()) }()

	// mock session
	oldSession := serverCreateSession
	serverCreateSession = func(ctx context.Context, c util.LexClient, input *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a"}, nil
	}
	defer func() { serverCreateSession = oldSession }()

	// httpGet returns image
	oldGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("img"))), Header: http.Header{"Content-Type": []string{"image/png"}}}, nil
	}
	defer func() { httpGet = oldGet }()

	// repoUploadBlob fails
	oldUpload := repoUploadBlob
	repoUploadBlob = func(ctx context.Context, c util.LexClient, input io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		return nil, errors.New("upload failed")
	}
	defer func() { repoUploadBlob = oldUpload }()

	calledCreate := false
	oldCreate := repoCreateRecord
	repoCreateRecord = func(ctx context.Context, c util.LexClient, input *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		calledCreate = true
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	defer func() { repoCreateRecord = oldCreate }()

	if err := PostToBluesky(context.Background(), "t", []string{"u"}, nil); err != nil {
		t.Fatalf("PostToBluesky returned error: %v", err)
	}
	if !calledCreate {
		t.Fatalf("expected RepoCreateRecord to be called even if upload failed")
	}
	// ensure hash saved
	b, _ := os.ReadFile(postedHashesFile)
	if len(b) == 0 {
		t.Fatalf("expected posted hash to be saved")
	}
}

func TestPostToBluesky_MultipleImages(t *testing.T) {
	tmp, _ := os.CreateTemp("", "posted-*.txt")
	_ = tmp.Close()
	oldPosted := postedHashesFile
	postedHashesFile = tmp.Name()
	defer func() { postedHashesFile = oldPosted; _ = os.Remove(tmp.Name()) }()

	oldSession := serverCreateSession
	serverCreateSession = func(ctx context.Context, c util.LexClient, input *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a"}, nil
	}
	defer func() { serverCreateSession = oldSession }()

	oldGet := httpGet
	httpGet = func(url string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("img"))), Header: http.Header{"Content-Type": []string{"image/png"}}}, nil
	}
	defer func() { httpGet = oldGet }()

	count := 0
	oldUpload := repoUploadBlob
	repoUploadBlob = func(ctx context.Context, c util.LexClient, input io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		count++
		return &atproto.RepoUploadBlob_Output{Blob: &util.LexBlob{MimeType: "image/png", Size: 3}}, nil
	}
	defer func() { repoUploadBlob = oldUpload }()

	oldCreate := repoCreateRecord
	repoCreateRecord = func(ctx context.Context, c util.LexClient, input *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	defer func() { repoCreateRecord = oldCreate }()

	if err := PostToBluesky(context.Background(), "t", []string{"u1", "u2", "u3"}, nil); err != nil {
		t.Fatalf("PostToBluesky returned error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 uploads, got %d", count)
	}
}

func TestTruncateText_ComplexGraphemes(t *testing.T) {
	// string with emoji sequences
	s := "👩‍👩‍👧‍👦" // family emoji (multiple graphemes)
	out := truncateText(s+"abcdef", 3)
	if len(out) == 0 {
		t.Fatalf("unexpected empty output")
	}
	// should end with ... when truncated
	if len(out) > 3 && out[len(out)-3:] != "..." {
		t.Fatalf("expected ellipsis, got %q", out)
	}
}

func TestPostToBluesky_SkipsNonImageDownloads(t *testing.T) {
	tmp, _ := os.CreateTemp("", "posted-*.txt")
	_ = tmp.Close()
	oldPosted := postedHashesFile
	postedHashesFile = tmp.Name()
	defer func() { postedHashesFile = oldPosted; _ = os.Remove(tmp.Name()) }()

	oldSession := serverCreateSession
	serverCreateSession = func(ctx context.Context, c util.LexClient, input *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a"}, nil
	}
	defer func() { serverCreateSession = oldSession }()

	// Instagram sometimes answers the media URL with an HTML page or an error.
	responses := map[string]*http.Response{
		"html":  {StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("<html>"))), Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}},
		"error": {StatusCode: 429, Body: io.NopCloser(bytes.NewReader([]byte("img"))), Header: http.Header{"Content-Type": []string{"image/jpeg"}}},
	}
	oldGet := httpGet
	httpGet = func(url string) (*http.Response, error) { return responses[url], nil }
	defer func() { httpGet = oldGet }()

	uploads := 0
	oldUpload := repoUploadBlob
	repoUploadBlob = func(ctx context.Context, c util.LexClient, input io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		uploads++
		return &atproto.RepoUploadBlob_Output{Blob: &util.LexBlob{MimeType: "*/*", Size: 6}}, nil
	}
	defer func() { repoUploadBlob = oldUpload }()

	var record *atproto.RepoCreateRecord_Input
	oldCreate := repoCreateRecord
	repoCreateRecord = func(ctx context.Context, c util.LexClient, input *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		record = input
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	defer func() { repoCreateRecord = oldCreate }()

	if err := PostToBluesky(context.Background(), "t", []string{"html", "error"}, nil); err != nil {
		t.Fatalf("PostToBluesky returned error: %v", err)
	}
	if uploads != 0 {
		t.Fatalf("non-image downloads must not be uploaded, got %d uploads", uploads)
	}
	if record == nil {
		t.Fatalf("expected the post to be published without images")
	}
}
