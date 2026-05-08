package app

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/x/is"
)

func TestFeedBuilderAddEntryDefaults(t *testing.T) {
	feed := NewFeed("test", "feed-id")
	feed.Add(FeedEntry{Title: "entry", Content: "body"})

	is.Equal(t, 1, len(feed.f.Entries))
	entry := feed.f.Entries[0]
	is.NotEqual(t, "", entry.ID)
	is.NotEqual(t, "", entry.Updated)
	is.Equal(t, 1, len(feed.f.Authors))
	is.Equal(t, "rss-tools", feed.f.Authors[0].Name)
}

func TestFeedBuilderBytesAndWriteTo(t *testing.T) {
	updated := time.Date(2026, 4, 20, 12, 30, 0, 0, time.UTC)
	feed := NewFeed("test", "feed-id").
		WithSubtitle("subtitle").
		Add(FeedEntry{Title: "entry", Content: "content", Updated: updated})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !bytes.Contains(raw, []byte("<subtitle>subtitle</subtitle>")) {
		t.Fatalf("expected subtitle in serialized feed")
	}

	var parsed AtomFeed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, "test", parsed.Title)
}

func TestFeedBuilderWithAuthor(t *testing.T) {
	feed := NewFeed("test", "feed-id").WithAuthor("moviefeed")
	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	var parsed AtomFeed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Authors))
	is.Equal(t, "moviefeed", parsed.Authors[0].Name)
}

func TestFeedBuilderRender(t *testing.T) {
	r := httptest.NewRecorder()
	err := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:   "entry",
			ID:      "entry-id",
			Content: "content",
			Updated: time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC),
		}).
		Render(r)
	is.Err(t, err, nil)

	is.Equal(t, http.StatusOK, r.Code)
	if got := r.Header().Get("Content-Type"); !strings.Contains(got, "application/atom+xml") {
		t.Fatalf("unexpected content type: %q", got)
	}
}

func TestFeedEntryTextContent(t *testing.T) {
	feed := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:       "text entry",
			Content:     "plain text content",
			ContentType: "text",
			Updated:     time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `<content type="text">plain text content</content>`) {
		t.Fatalf("expected text content with type attribute in serialized feed")
	}

	var parsed AtomFeed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Entries))

	entry := parsed.Entries[0]
	is.Equal(t, "text", entry.Content.Type)
	is.Equal(t, "plain text content", entry.Content.Value)
}

func TestFeedEntryHtmlContent(t *testing.T) {
	htmlContent := "<p>Hello <strong>World</strong></p>"
	feed := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:       "html entry",
			Content:     htmlContent,
			ContentType: "html",
			Updated:     time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `<content type="html">`) {
		t.Fatalf("expected HTML content with type='html' attribute in serialized feed")
	}

	var parsed AtomFeed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Entries))

	entry := parsed.Entries[0]
	is.Equal(t, "html", entry.Content.Type)
	is.Equal(t, htmlContent, entry.Content.Value)
}

func TestFeedEntryXHTMLContent(t *testing.T) {
	xhtmlContent := `<body><p>Hello <strong>World</strong></p></body>`
	feed := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:       "xhtml entry",
			Content:     xhtmlContent,
			ContentType: "xhtml",
			Updated:     time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `<content type="xhtml">`) {
		t.Fatalf("expected XHTML content with type='xhtml' attribute in serialized feed")
	}
	if !strings.Contains(string(raw), `<div xmlns="http://www.w3.org/1999/xhtml"><body><p>Hello <strong>World</strong></p></body></div>`) {
		t.Fatalf("expected XHTML div wrapper for content")
	}
}

func TestFeedEntryLinks(t *testing.T) {
	feed := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:   "entry",
			Content: "hello",
			Links: []FeedLink{
				{Rel: "alternate", Type: "text/html", Href: "https://example.com/item"},
			},
			Updated: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `<link rel="alternate" type="text/html" href="https://example.com/item"></link>`) {
		t.Fatalf("expected link element in serialized feed")
	}

	var parsed AtomFeed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Entries))
	is.Equal(t, 1, len(parsed.Entries[0].Links))
	is.Equal(t, "https://example.com/item", parsed.Entries[0].Links[0].Href)
}

func TestFeedEntryLinksWithLength(t *testing.T) {
	feed := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:   "entry",
			Content: "hello",
			Links: []FeedLink{
				{Rel: "enclosure", Type: "image/jpeg", Length: "0", Href: "https://example.com/item.jpg"},
			},
			Updated: time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `rel="enclosure" type="image/jpeg" length="0" href="https://example.com/item.jpg"`) {
		t.Fatalf("expected enclosure link with length in serialized feed")
	}
}

func TestFeedMultipleEntriesWithMixedContentTypes(t *testing.T) {
	updated := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	feed := NewFeed("test", "feed-id").
		Add(FeedEntry{
			Title:       "text entry",
			Content:     "plain text",
			ContentType: "text",
			Updated:     updated,
		}).
		Add(FeedEntry{
			Title:       "html entry",
			Content:     "<p>html content</p>",
			ContentType: "html",
			Updated:     updated,
		}).
		Add(FeedEntry{
			Title:   "default entry",
			Content: "default content",
			Updated: updated,
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)

	var parsed AtomFeed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 3, len(parsed.Entries))

	tests := []struct {
		name         string
		expectedType string
		expectedText string
	}{
		{"text entry", "text", "plain text"},
		{"html entry", "html", "<p>html content</p>"},
		{"default entry", "text", "default content"},
	}
	for i, tc := range tests {
		is.Equal(t, tc.name, parsed.Entries[i].Title)
		is.Equal(t, tc.expectedText, parsed.Entries[i].Content.Value)
		is.Equal(t, tc.expectedType, parsed.Entries[i].Content.Type)
	}
}
