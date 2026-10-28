package socialfeed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Mundo-Dolphins/dolphins-alerts/rss"
)

type Platform string

const PlatformTwitterX Platform = "twitter_x"

var (
	ErrInvalidHandle = errors.New("invalid twitter handle")
	ErrInvalidItem   = errors.New("invalid twitter rss item")
)

type SocialPost struct {
	ID          string
	Platform    Platform
	Account     string
	Author      string
	Text        string
	URL         string
	Images      []string
	Videos      []string
	PublishedAt time.Time
}

type Source struct {
	rss     rss.Client
	baseURL string
}

func NewSource(baseURL string, client rss.Client) *Source {
	if client == nil {
		client = rss.NewClient(30 * time.Second)
	}
	return &Source{
		rss:     client,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
}

func (s *Source) Fetch(ctx context.Context, handle string) ([]SocialPost, error) {
	account, err := NormalizeHandle(handle)
	if err != nil {
		return nil, err
	}
	if s.baseURL == "" {
		return nil, fmt.Errorf("twitter rss base url is empty")
	}

	feed, err := s.rss.Fetch(ctx, s.feedURL(account))
	if err != nil {
		return nil, err
	}
	if len(feed.Items) == 0 {
		return []SocialPost{}, nil
	}

	posts := make([]SocialPost, 0, len(feed.Items))
	for _, item := range feed.Items {
		post, err := itemToPost(account, item)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, nil
}

func (s *Source) feedURL(handle string) string {
	return s.baseURL + "/user/" + url.PathEscape(handle)
}

func NormalizeHandle(handle string) (string, error) {
	handle = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	if handle == "" {
		return "", ErrInvalidHandle
	}
	if strings.ContainsAny(handle, `/\?#:%`) {
		return "", fmt.Errorf("%w: %q", ErrInvalidHandle, handle)
	}
	for _, r := range handle {
		if r <= 31 || r == 127 || !(r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return "", fmt.Errorf("%w: %q", ErrInvalidHandle, handle)
		}
	}
	return strings.ToLower(handle), nil
}

func itemToPost(account string, item rss.Item) (SocialPost, error) {
	postURL := strings.TrimSpace(item.Link)
	guid := strings.TrimSpace(item.GUID)
	if guid == "" {
		guid = postURL
	}
	id := ExtractStatusID(postURL)
	if id == "" {
		id = ExtractStatusID(guid)
	}
	if id == "" {
		id = guid
	}
	if strings.TrimSpace(id) == "" {
		return SocialPost{}, fmt.Errorf("%w: missing guid/link", ErrInvalidItem)
	}

	text := CleanTwitterText(item.Description)
	if text == "" {
		text = CleanTwitterText(item.Title)
	}

	return SocialPost{
		ID:          id,
		Platform:    PlatformTwitterX,
		Account:     account,
		Author:      strings.TrimSpace(item.Author),
		Text:        text,
		URL:         postURL,
		Images:      enclosureURLs(item.Enclosures, "image/"),
		Videos:      enclosureURLs(item.Enclosures, "video/"),
		PublishedAt: item.PublishedAt,
	}, nil
}

// enclosureURLs returns the attachments whose MIME type starts with prefix.
func enclosureURLs(enclosures []rss.Enclosure, prefix string) []string {
	var urls []string
	for _, e := range enclosures {
		if strings.HasPrefix(strings.ToLower(e.Type), prefix) {
			urls = append(urls, e.URL)
		}
	}
	return urls
}

var statusIDPattern = regexp.MustCompile(`/status/([0-9]+)`)

func ExtractStatusID(value string) string {
	matches := statusIDPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func CleanTwitterText(text string) string {
	decoded := html.UnescapeString(text)
	decoded = html.UnescapeString(decoded)
	decoded = stripHTMLTags(decoded)
	return normalizeWhitespace(decoded)
}

func stripHTMLTags(text string) string {
	var b strings.Builder
	inTag := false
	for _, r := range text {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func normalizeWhitespace(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, line)
		blank = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
