package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Fatalf("expected user agent")
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel><title>Feed</title><item><title>One</title><link>https://example.com/1</link><guid>1</guid><description>Desc</description><pubDate>Tue, 08 Sep 2026 10:00:00 GMT</pubDate></item></channel></rss>`))
	}))
	defer server.Close()

	feed, err := NewClient(time.Second).Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if feed.Title != "Feed" || len(feed.Items) != 1 || feed.Items[0].GUID != "1" {
		t.Fatalf("unexpected feed: %+v", feed)
	}
}

func TestHTTPClientFetchStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad upstream"))
	}))
	defer server.Close()

	_, err := NewClient(time.Second).Fetch(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("expected status error, got %v", err)
	}
}
