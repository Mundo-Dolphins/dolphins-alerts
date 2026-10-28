package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	util "github.com/bluesky-social/indigo/lex/util"
)

// noisyPNG builds an incompressible image so it is far larger than the blob limit.
func noisyPNG(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256))
		img.Pix[i+3] = 255
		if alpha {
			img.Pix[i+3] = 0
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFitImage_SmallImageIsUntouched(t *testing.T) {
	in := []byte("tiny")
	out, err := fitImage(in, maxBlobBytes)
	if err != nil || !bytes.Equal(in, out) {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestFitImage_ShrinksOversizedImageUnderLimit(t *testing.T) {
	in := noisyPNG(t, 1600, 1200, false)
	if len(in) <= maxBlobBytes {
		t.Fatalf("test image too small: %d", len(in))
	}
	out, err := fitImage(in, maxBlobBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > maxBlobBytes {
		t.Fatalf("result is %d bytes, limit %d", len(out), maxBlobBytes)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("result must be a valid JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() > 1600 || b.Dy() > 1200 || b.Dx()*3 != b.Dy()*4 && abs(b.Dx()*3-b.Dy()*4) > 8 {
		t.Fatalf("unexpected size/aspect ratio: %v", b)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestFitImage_TransparencyBecomesWhiteNotBlack(t *testing.T) {
	in := noisyPNG(t, 1500, 1500, true) // fully transparent
	out, err := fitImage(in, maxBlobBytes/2)
	if err != nil {
		t.Fatal(err)
	}
	img, _ := jpeg.Decode(bytes.NewReader(out))
	r, g, b, _ := img.At(10, 10).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Fatalf("transparent pixels must flatten onto white, got %v", color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255})
	}
}

func TestFitImage_UndecodableOversizedIsAnError(t *testing.T) {
	if _, err := fitImage(make([]byte, maxBlobBytes+10), maxBlobBytes); err == nil {
		t.Fatal("expected an error for data that is not an image")
	}
}

func stubSession(t *testing.T) {
	t.Helper()
	old := serverCreateSession
	serverCreateSession = func(context.Context, util.LexClient, *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
		return &atproto.ServerCreateSession_Output{AccessJwt: "a", RefreshJwt: "r", Handle: "h", Did: "did:plc:me"}, nil
	}
	t.Cleanup(func() { serverCreateSession = old })
	oldPosted := postedHashesFile
	postedHashesFile = t.TempDir() + "/hashes.txt"
	t.Cleanup(func() { postedHashesFile = oldPosted })
}

func captureRecord(t *testing.T) **appbsky.FeedPost {
	t.Helper()
	var got *appbsky.FeedPost
	old := repoCreateRecord
	repoCreateRecord = func(_ context.Context, _ util.LexClient, in *atproto.RepoCreateRecord_Input) (*atproto.RepoCreateRecord_Output, error) {
		got, _ = in.Record.Val.(*appbsky.FeedPost)
		return &atproto.RepoCreateRecord_Output{}, nil
	}
	t.Cleanup(func() { repoCreateRecord = old })
	return &got
}

const testBlobJSON = `{"$type":"blob","ref":{"$link":"bafkreibme22gw2h7y2h7tg2fhqotaqjucnbc24deqo72b6mkl2egezxhvy"},"mimeType":"video/mp4","size":9}`

// fakeVideoServices serves the tweet video and a stand-in for video.bsky.app.
func fakeVideoServices(t *testing.T, finalState string) (videoURL string, polls *int32) {
	t.Helper()
	polls = new(int32)
	mux := http.NewServeMux()
	mux.HandleFunc("/clip.mp4", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("mp4-bytes"))
	})
	mux.HandleFunc("/xrpc/app.bsky.video.uploadVideo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer svc-token" || r.Header.Get("Content-Type") != "video/mp4" ||
			r.URL.Query().Get("did") != "did:plc:me" || r.URL.Query().Get("name") == "" || string(body) != "mp4-bytes" {
			t.Errorf("bad upload request: auth=%q ct=%q query=%v body=%q", r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.URL.Query(), body)
		}
		// the official guide reads the JobStatus straight from this response
		_ = json.NewEncoder(w).Encode(map[string]any{"jobId": "job1", "did": "did:plc:me", "state": "JOB_STATE_CREATED"})
	})
	mux.HandleFunc("/xrpc/app.bsky.video.getJobStatus", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("jobId") != "job1" {
			t.Errorf("jobId = %q", r.URL.Query().Get("jobId"))
		}
		status := map[string]any{"jobId": "job1", "did": "did:plc:me", "state": "JOB_STATE_PROCESSING"}
		if atomic.AddInt32(polls, 1) >= 2 {
			status["state"] = finalState
			if finalState == "JOB_STATE_COMPLETED" {
				status["blob"] = json.RawMessage(testBlobJSON)
			} else {
				status["error"], status["message"] = "video_too_long", "too long"
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jobStatus": status})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oldURL, oldAuth, oldPoll := videoServiceURL, serverGetServiceAuth, videoPollInterval
	videoServiceURL, videoPollInterval = srv.URL, time.Millisecond
	serverGetServiceAuth = func(_ context.Context, _ util.LexClient, aud string, exp int64, lxm string) (*atproto.ServerGetServiceAuth_Output, error) {
		if aud != "did:web:bsky.social" || lxm != "com.atproto.repo.uploadBlob" || exp <= time.Now().Unix() {
			t.Errorf("bad service auth request aud=%q lxm=%q exp=%d", aud, lxm, exp)
		}
		return &atproto.ServerGetServiceAuth_Output{Token: "svc-token"}, nil
	}
	t.Cleanup(func() { videoServiceURL, serverGetServiceAuth, videoPollInterval = oldURL, oldAuth, oldPoll })
	return srv.URL + "/clip.mp4", polls
}

func TestPostToBluesky_UploadsVideoThroughVideoService(t *testing.T) {
	stubSession(t)
	rec := captureRecord(t)
	videoURL, polls := fakeVideoServices(t, "JOB_STATE_COMPLETED")

	if err := PostToBluesky(context.Background(), "Mira esto", nil, []string{videoURL}); err != nil {
		t.Fatal(err)
	}
	got := *rec
	if got == nil || got.Embed == nil || got.Embed.EmbedVideo == nil || got.Embed.EmbedVideo.Video == nil {
		t.Fatalf("expected a video embed, got %+v", got)
	}
	if got.Embed.EmbedVideo.Video.MimeType != "video/mp4" {
		t.Fatalf("blob = %+v", got.Embed.EmbedVideo.Video)
	}
	if atomic.LoadInt32(polls) < 2 {
		t.Fatalf("expected to poll until the job completed, polls=%d", *polls)
	}
}

func TestPostToBluesky_VideoFailureFallsBackToTextAndImages(t *testing.T) {
	stubSession(t)
	rec := captureRecord(t)
	videoURL, _ := fakeVideoServices(t, "JOB_STATE_FAILED")

	oldGet, oldUpload := httpGet, repoUploadBlob
	httpGet = func(string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("img")), Header: http.Header{"Content-Type": []string{"image/png"}}}, nil
	}
	repoUploadBlob = func(context.Context, util.LexClient, io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		return &atproto.RepoUploadBlob_Output{Blob: &util.LexBlob{MimeType: "image/png", Size: 3}}, nil
	}
	defer func() { httpGet, repoUploadBlob = oldGet, oldUpload }()

	if err := PostToBluesky(context.Background(), "Mira esto", []string{"http://x/i.png"}, []string{videoURL}); err != nil {
		t.Fatalf("a failed video must not lose the post: %v", err)
	}
	got := *rec
	if got == nil || got.Embed == nil || got.Embed.EmbedVideo != nil || got.Embed.EmbedImages == nil {
		t.Fatalf("expected fallback to the image embed, got %+v", got)
	}
}

func TestPostToBluesky_ShrinksOversizedImageBeforeUpload(t *testing.T) {
	stubSession(t)
	captureRecord(t)
	big := noisyPNG(t, 1600, 1200, false)

	oldGet, oldUpload := httpGet, repoUploadBlob
	httpGet = func(string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(big)), Header: http.Header{"Content-Type": []string{"image/png"}}}, nil
	}
	var uploaded int
	repoUploadBlob = func(_ context.Context, _ util.LexClient, in io.Reader) (*atproto.RepoUploadBlob_Output, error) {
		b, _ := io.ReadAll(in)
		uploaded = len(b)
		return &atproto.RepoUploadBlob_Output{Blob: &util.LexBlob{MimeType: "image/jpeg", Size: int64(len(b))}}, nil
	}
	defer func() { httpGet, repoUploadBlob = oldGet, oldUpload }()

	if err := PostToBluesky(context.Background(), "foto grande", []string{"http://x/big.png"}, nil); err != nil {
		t.Fatal(err)
	}
	if uploaded == 0 || uploaded > maxBlobBytes {
		t.Fatalf("uploaded %d bytes, want 1..%d", uploaded, maxBlobBytes)
	}
}

func TestParseJobStatus_AcceptsFlatAndWrappedShapes(t *testing.T) {
	for name, body := range map[string]string{
		"flat":    `{"jobId":"j","state":"JOB_STATE_CREATED"}`,
		"wrapped": `{"jobStatus":{"jobId":"j","state":"JOB_STATE_CREATED"}}`,
		// already_exists comes back as an error that still names the job
		"already_exists": `{"error":"already_exists","jobId":"j","state":"JOB_STATE_COMPLETED"}`,
	} {
		st, err := parseJobStatus([]byte(body))
		if err != nil || st.JobId != "j" {
			t.Errorf("%s: status=%+v err=%v", name, st, err)
		}
	}
	if _, err := parseJobStatus([]byte(`{"error":"Unauthorized"}`)); err == nil {
		t.Error("a response without a job id must be an error")
	}
}
