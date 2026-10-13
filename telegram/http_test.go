package telegram

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSendTelegramMessage_Success(t *testing.T) {
	// Start a test server to receive the POST
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// validate path contains sendMessage
		if !strings.Contains(r.URL.Path, "/sendMessage") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if len(b) == 0 {
			t.Fatalf("empty body")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	// Replace client to route requests to test server using a RoundTripper that rewrites host
	oldClient := httpClient
	defer func() { httpClient = oldClient }()

	httpClient = &http.Client{Transport: roundTripToServer(ts)}

	chatID := "12345"
	message := "hello"

	err := SendTelegramMessage("TOKEN", chatID, message)
	if err != nil {
		t.Fatalf("SendTelegramMessage failed: %v", err)
	}
}

func TestSendTelegramMessage_HTTPError(t *testing.T) {
	// Return non-200
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer ts.Close()

	oldClient := httpClient
	defer func() { httpClient = oldClient }()

	httpClient = &http.Client{Transport: roundTripToServer(ts)}

	err := SendTelegramMessage("TOKEN", "chat", "msg")
	if err == nil {
		t.Fatalf("expected error for non-200 response")
	}
}

func TestSendHTMLMessagePreservesSpoilerAndFormValues(t *testing.T) {
	message := "🎯 ¡ANOTACIÓN!\n\n<tg-spoiler>A &amp; B + 6-0</tg-spoiler>"
	for _, mode := range []string{"HTML", "Markdown"} {
		t.Run(mode, func(t *testing.T) {
			old := httpClient
			t.Cleanup(func() { httpClient = old })
			httpClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				if err := request.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if request.Form.Get("parse_mode") != mode || request.Form.Get("text") != message || request.Form.Get("chat_id") != "chat+id" {
					t.Fatalf("invalid form: %v", request.Form)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
			})}
			var err error
			if mode == "HTML" {
				err = SendHTMLMessage("test-token", "chat+id", message)
			} else {
				err = SendTelegramMessage("test-token", "chat+id", message)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// roundTripToServer rewrites request URLs to the test server host so calls to
// https://api.telegram.org/... will be issued against ts.
func roundTripToServer(ts *httptest.Server) http.RoundTripper {
	u, _ := url.Parse(ts.URL)
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// clone request to avoid mutating caller's request
		r2 := new(http.Request)
		*r2 = *req
		r2.URL = new(url.URL)
		*r2.URL = *req.URL
		r2.URL.Scheme = u.Scheme
		r2.URL.Host = u.Host
		r2.Host = u.Host
		return http.DefaultTransport.RoundTrip(r2)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendTelegramMessage_ErrorDoesNotLeakToken(t *testing.T) {
	oldClient := httpClient
	defer func() { httpClient = oldClient }()
	// http.Client wraps transport errors in a *url.Error that embeds the full URL.
	httpClient = &http.Client{Transport: failingTransport{}}
	err := SendTelegramMessage("123456:SECRET-TOKEN-VALUE", "1", "hello")
	if err == nil {
		t.Fatal("expected a network error")
	}
	if strings.Contains(err.Error(), "SECRET-TOKEN-VALUE") {
		t.Fatalf("error leaks the bot token: %v", err)
	}
}

func TestHTTPClientHasTimeout(t *testing.T) {
	if httpClient.Timeout == 0 {
		t.Fatal("default Telegram HTTP client must have a timeout")
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network down")
}
