package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // register decoders for image.Decode
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	util "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
)

const (
	// maxBlobBytes is the blob size Bluesky accepts for an image.
	maxBlobBytes = 1_000_000
	// maxImageDownloadBytes bounds what is downloaded before shrinking it.
	maxImageDownloadBytes = 20 << 20
	// maxImagePixels guards against decompression bombs.
	maxImagePixels = 50_000_000
	// maxVideoBytes is the largest video the Bluesky video service accepts.
	maxVideoBytes = 100 << 20
)

// injection points for testing
var (
	videoServiceURL      = "https://video.bsky.app"
	videoPollInterval    = time.Second
	videoPollTimeout     = 5 * time.Minute
	serverGetServiceAuth = atproto.ServerGetServiceAuth
	// videoClient is used for downloading and uploading videos, which are far
	// bigger than images and need a much longer timeout than mediaClient.
	videoClient = &http.Client{Timeout: 10 * time.Minute}
)

// fitImage returns data unchanged when it already fits in limit bytes.
// Otherwise it re-encodes it as a JPEG, lowering the quality and then the
// resolution until it fits.
func fitImage(data []byte, limit int) ([]byte, error) {
	if len(data) <= limit {
		return data, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image is %d bytes (limit %d) and cannot be resized: %w", len(data), limit, err)
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return nil, fmt.Errorf("image is too large to resize: %dx%d", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	flat := flattenOnWhite(src)

	for scale := 1.0; scale >= 0.2; scale *= 0.8 {
		img := image.Image(flat)
		if scale < 1 {
			img = scaleDown(flat, scale)
		}
		for _, quality := range []int{85, 75, 65, 50} {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
				return nil, err
			}
			if buf.Len() <= limit {
				return buf.Bytes(), nil
			}
		}
	}
	return nil, fmt.Errorf("could not shrink image below %d bytes", limit)
}

// flattenOnWhite drops transparency (JPEG has none) onto a white background.
func flattenOnWhite(src image.Image) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Over)
	return dst
}

// scaleDown shrinks src by averaging the source pixels under each target pixel.
func scaleDown(src *image.RGBA, scale float64) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := max(1, int(float64(sw)*scale)), max(1, int(float64(sh)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := y*sh/dh, max(y*sh/dh+1, (y+1)*sh/dh)
		for x := 0; x < dw; x++ {
			x0, x1 := x*sw/dw, max(x*sw/dw+1, (x+1)*sw/dw)
			var r, g, b, n int
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride:]
				for sx := x0; sx < x1; sx++ {
					r, g, b, n = r+int(row[sx*4]), g+int(row[sx*4+1]), b+int(row[sx*4+2]), n+1
				}
			}
			o := y*dst.Stride + x*4
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return dst
}

// uploadImage downloads imgURL, shrinks it if needed and uploads it as a blob.
func uploadImage(ctx context.Context, client *xrpc.Client, imgURL string) (*util.LexBlob, error) {
	log.Printf("Downloading image: %s", imgURL)
	resp, err := httpGet(imgURL)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	mimeType := resp.Header.Get("Content-Type")
	// Bluesky rejects the whole post if an embedded blob is not an image.
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(mimeType, "image/") {
		return nil, fmt.Errorf("status=%d MIME=%s", resp.StatusCode, mimeType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(data) > maxImageDownloadBytes {
		return nil, fmt.Errorf("image is larger than %d bytes", maxImageDownloadBytes)
	}
	if fitted, err := fitImage(data, maxBlobBytes); err != nil {
		return nil, err
	} else if len(fitted) != len(data) {
		log.Printf("Image %s resized from %d to %d bytes", imgURL, len(data), len(fitted))
		data = fitted
	}

	blobResp, err := repoUploadBlob(ctx, client, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("upload blob: %w", err)
	}
	log.Printf("Blob uploaded successfully: Ref=%s, MIME=%s", blobResp.Blob.Ref, mimeType)
	return blobResp.Blob, nil
}

// parseJobStatus reads a video job status. The video service answers either
// with the status itself or wrapped in {"jobStatus": ...}, and an
// "already_exists" error still names the job, so all three are accepted.
func parseJobStatus(body []byte) (*appbsky.VideoDefs_JobStatus, error) {
	var env struct {
		appbsky.VideoDefs_JobStatus
		Wrapped *appbsky.VideoDefs_JobStatus `json:"jobStatus"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("invalid job status %q: %w", truncateBody(body), err)
	}
	st := &env.VideoDefs_JobStatus
	if env.Wrapped != nil {
		st = env.Wrapped
	}
	if st.JobId == "" {
		return nil, fmt.Errorf("no job id in response %q", truncateBody(body))
	}
	return st, nil
}

func truncateBody(b []byte) string {
	if len(b) > 300 {
		b = b[:300]
	}
	return string(b)
}

// uploadVideo sends a video to the Bluesky video service and waits for its
// processing job to produce the blob to embed. Flow documented in
// https://docs.bsky.app/docs/tutorials/video
func uploadVideo(ctx context.Context, client *xrpc.Client, did, videoURL string) (*util.LexBlob, error) {
	file, size, err := downloadVideo(ctx, videoURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()

	host, err := url.Parse(client.Host)
	if err != nil || host.Host == "" {
		return nil, fmt.Errorf("invalid PDS host %q", client.Host)
	}
	auth, err := serverGetServiceAuth(ctx, client, "did:web:"+host.Host, time.Now().Add(30*time.Minute).Unix(), "com.atproto.repo.uploadBlob")
	if err != nil {
		return nil, fmt.Errorf("service auth: %w", err)
	}

	name := path.Base(mustPath(videoURL))
	if name == "." || name == "/" || name == "" {
		name = "video.mp4"
	}
	q := url.Values{"did": {did}, "name": {name}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, videoServiceURL+"/xrpc/app.bsky.video.uploadVideo?"+q.Encode(), file)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	req.Header.Set("Content-Type", "video/mp4")
	resp, err := videoClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload video: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	st, err := parseJobStatus(body)
	if err != nil {
		return nil, fmt.Errorf("upload video (HTTP %d): %w", resp.StatusCode, err)
	}

	ctx, cancel := context.WithTimeout(ctx, videoPollTimeout)
	defer cancel()
	for {
		if st.Blob != nil {
			return st.Blob, nil
		}
		if st.Error != nil || st.State == "JOB_STATE_FAILED" {
			return nil, fmt.Errorf("video job %s failed: state=%s error=%s message=%s", st.JobId, st.State, deref(st.Error), deref(st.Message))
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("video job %s did not finish: %w", st.JobId, ctx.Err())
		case <-time.After(videoPollInterval):
		}
		if st, err = fetchJobStatus(ctx, st.JobId); err != nil {
			return nil, err
		}
	}
}

func fetchJobStatus(ctx context.Context, jobID string) (*appbsky.VideoDefs_JobStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoServiceURL+"/xrpc/app.bsky.video.getJobStatus?"+url.Values{"jobId": {jobID}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := videoClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("job status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job status HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}
	return parseJobStatus(body)
}

// downloadVideo stores the video in a temporary file (it can be large) and
// returns it rewound, with its size.
func downloadVideo(ctx context.Context, videoURL string) (*os.File, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := videoClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("download video: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if mt := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusOK || !strings.HasPrefix(mt, "video/") {
		return nil, 0, fmt.Errorf("download video: status=%d MIME=%s", resp.StatusCode, mt)
	}

	f, err := os.CreateTemp("", "bsky-video-*.mp4")
	if err != nil {
		return nil, 0, err
	}
	fail := func(err error) (*os.File, int64, error) {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, 0, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxVideoBytes+1))
	if err != nil {
		return fail(fmt.Errorf("download video: %w", err))
	}
	if n > maxVideoBytes {
		return fail(errors.New("video is larger than 100 MB"))
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return f, n, nil
}

func mustPath(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Path
	}
	return ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
