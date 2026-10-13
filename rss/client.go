package rss

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	userAgent = "dolphins-alerts (+https://github.com/Mundo-Dolphins/dolphins-alerts)"
	// maxFeedBytes bounds how much of a feed is read.
	maxFeedBytes = 8 << 20
)

var (
	ErrProxyUnavailable = errors.New("rss proxy unavailable")
	ErrHTTPStatus       = errors.New("rss http status error")
	ErrInvalidXML       = errors.New("invalid rss xml")
	ErrInvalidPubDate   = errors.New("invalid rss pubDate")
)

type Item struct {
	GUID        string
	Title       string
	Description string
	Link        string
	Author      string
	DateRaw     string
	PubDate     string
	PublishedAt time.Time
}

type Feed struct {
	Title string
	Items []Item
}

type Client interface {
	Fetch(ctx context.Context, url string) (Feed, error)
}

type HTTPClient struct {
	client *http.Client
}

func NewClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Fetch(ctx context.Context, url string) (Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Feed{}, fmt.Errorf("create RSS request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return Feed{}, fmt.Errorf("%w: %v", ErrProxyUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Feed{}, fmt.Errorf("%w: status %d, body: %s", ErrHTTPStatus, resp.StatusCode, string(body)[:min(500, len(body))])
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return Feed{}, fmt.Errorf("read RSS response: %w", err)
	}

	var raw rssDocument
	if err := xml.Unmarshal(body, &raw); err != nil {
		return Feed{}, fmt.Errorf("%w: %v", ErrInvalidXML, err)
	}

	feed, err := raw.toFeed()
	if err != nil {
		return Feed{}, err
	}

	return feed, nil
}

type rssDocument struct {
	XMLName xml.Name
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	AtomTitle   string      `xml:"title"`
	AtomEntries []atomEntry `xml:"entry"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
	Author      string `xml:"author"`
}

type atomEntry struct {
	Title   string `xml:"title"`
	ID      string `xml:"id"`
	Summary string `xml:"summary"`
	Content string `xml:"content"`
	Updated string `xml:"updated"`
	Author  struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
}

func (d rssDocument) toFeed() (Feed, error) {
	switch strings.ToLower(d.XMLName.Local) {
	case "rss":
		return d.rssFeed()
	case "feed":
		return d.atomFeed()
	default:
		return Feed{}, fmt.Errorf("%w: unsupported root %q", ErrInvalidXML, d.XMLName.Local)
	}
}

func (d rssDocument) rssFeed() (Feed, error) {
	items := make([]Item, 0, len(d.Channel.Items))
	for _, raw := range d.Channel.Items {
		publishedAt, err := parseFeedTime(raw.PubDate)
		if err != nil {
			return Feed{}, err
		}
		items = append(items, Item{
			GUID:        strings.TrimSpace(raw.GUID),
			Title:       strings.TrimSpace(raw.Title),
			Description: raw.Description,
			Link:        strings.TrimSpace(raw.Link),
			Author:      strings.TrimSpace(raw.Author),
			DateRaw:     strings.TrimSpace(raw.PubDate),
			PubDate:     strings.TrimSpace(raw.PubDate),
			PublishedAt: publishedAt,
		})
	}
	return Feed{Title: strings.TrimSpace(d.Channel.Title), Items: items}, nil
}

func (d rssDocument) atomFeed() (Feed, error) {
	items := make([]Item, 0, len(d.AtomEntries))
	for _, raw := range d.AtomEntries {
		publishedAt, err := parseFeedTime(raw.Updated)
		if err != nil {
			return Feed{}, err
		}
		description := raw.Summary
		if description == "" {
			description = raw.Content
		}
		items = append(items, Item{
			GUID:        strings.TrimSpace(raw.ID),
			Title:       strings.TrimSpace(raw.Title),
			Description: description,
			Link:        atomLink(raw),
			Author:      strings.TrimSpace(raw.Author.Name),
			DateRaw:     strings.TrimSpace(raw.Updated),
			PubDate:     strings.TrimSpace(raw.Updated),
			PublishedAt: publishedAt,
		})
	}
	return Feed{Title: strings.TrimSpace(d.AtomTitle), Items: items}, nil
}

func atomLink(entry atomEntry) string {
	var fallback string
	for _, link := range entry.Links {
		if fallback == "" {
			fallback = strings.TrimSpace(link.Href)
		}
		if link.Rel == "" || link.Rel == "alternate" {
			return strings.TrimSpace(link.Href)
		}
	}
	return fallback
}

func parseFeedTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		time.RFC3339Nano,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidPubDate, value)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
