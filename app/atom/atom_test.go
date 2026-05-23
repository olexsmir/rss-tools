package atom

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/x/is"
)

func TestNewFeedDefaults(t *testing.T) {
	feed := NewFeed("test", "feed-id")

	is.Equal(t, "test", feed.Title)
	is.Equal(t, "feed-id", feed.ID)
	is.NotEqual(t, "", feed.Updated)
	is.Equal(t, 1, len(feed.Author))
	is.Equal(t, "rss-tools", feed.Author[0].Name)
}

func TestFeedAddAppendsEntry(t *testing.T) {
	feed := NewFeed("test", "feed-id")
	entry := &Entry{
		Title:   "entry",
		ID:      "entry-id",
		Updated: Time(time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)),
		Content: NewText("body", ""),
	}
	feed.Add(entry)

	is.Equal(t, 1, len(feed.Entry))
	is.Equal(t, "entry-id", feed.Entry[0].ID)
}

func TestFeedBytesAndWriteTo(t *testing.T) {
	updated := time.Date(2026, 4, 20, 12, 30, 0, 0, time.UTC)
	feed := NewFeed("test", "feed-id").
		Add(&Entry{
			Title:   "entry",
			ID:      "entry-id",
			Updated: Time(updated),
			Content: NewText("content", ""),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)

	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, "test", parsed.Title)
}

func TestFeedWithAuthor(t *testing.T) {
	feed := NewFeed("test", "feed-id").WithAuthor("moviefeed")
	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Author))
	is.Equal(t, "moviefeed", parsed.Author[0].Name)
}

func TestFeedRender(t *testing.T) {
	r := httptest.NewRecorder()
	err := NewFeed("test", "feed-id").
		Add(&Entry{
			Title:   "entry",
			ID:      "entry-id",
			Content: NewText("content", ""),
			Updated: Time(time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)),
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
		Add(&Entry{
			Title:   "text entry",
			ID:      "entry-id",
			Content: NewText("plain text content", "text"),
			Updated: Time(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `<content type="text">plain text content</content>`) {
		t.Fatalf("expected text content with type attribute in serialized feed")
	}

	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Entry))

	entry := parsed.Entry[0]
	if entry.Content == nil {
		t.Fatalf("expected content element in entry")
	}
	is.Equal(t, "text", entry.Content.Type)
	is.Equal(t, "plain text content", entry.Content.Body)
}

func TestFeedEntryHtmlContent(t *testing.T) {
	htmlContent := "<p>Hello <strong>World</strong></p>"
	feed := NewFeed("test", "feed-id").
		Add(&Entry{
			Title:   "html entry",
			ID:      "entry-id",
			Content: NewText(htmlContent, "html"),
			Updated: Time(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	if !strings.Contains(string(raw), `<content type="html">`) {
		t.Fatalf("expected HTML content with type='html' attribute in serialized feed")
	}

	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Entry))

	entry := parsed.Entry[0]
	if entry.Content == nil {
		t.Fatalf("expected content element in entry")
	}
	is.Equal(t, "html", entry.Content.Type)
	is.Equal(t, htmlContent, entry.Content.Body)
}

func TestFeedEntryXHTMLContent(t *testing.T) {
	xhtmlContent := `<body><p>Hello <strong>World</strong></p></body>`
	feed := NewFeed("test", "feed-id").
		Add(&Entry{
			Title:   "xhtml entry",
			ID:      "entry-id",
			Content: NewText(xhtmlContent, "xhtml"),
			Updated: Time(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)),
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
		Add(&Entry{
			Title:   "entry",
			ID:      "entry-id",
			Content: NewText("hello", ""),
			Link: []Link{
				{Rel: "alternate", Type: "text/html", Href: "https://example.com/item"},
			},
			Updated: Time(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)

	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 1, len(parsed.Entry))
	is.Equal(t, 1, len(parsed.Entry[0].Link))
	is.Equal(t, "https://example.com/item", parsed.Entry[0].Link[0].Href)
}

func TestFeedEntryLinksWithLength(t *testing.T) {
	feed := NewFeed("test", "feed-id").
		Add(&Entry{
			Title:   "entry",
			ID:      "entry-id",
			Content: NewText("hello", ""),
			Link: []Link{
				{Rel: "enclosure", Type: "image/jpeg", Length: 0, Href: "https://example.com/item.jpg"},
			},
			Updated: Time(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)
	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	if len(parsed.Entry) == 0 || len(parsed.Entry[0].Link) == 0 {
		t.Fatalf("expected enclosure link in parsed feed")
	}
	is.Equal(t, uint(0), parsed.Entry[0].Link[0].Length)
}

func TestFeedMultipleEntriesWithMixedContentTypes(t *testing.T) {
	updated := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	feed := NewFeed("test", "feed-id").
		Add(&Entry{
			Title:   "text entry",
			ID:      "entry-text",
			Content: NewText("plain text", "text"),
			Updated: Time(updated),
		}).
		Add(&Entry{
			Title:   "html entry",
			ID:      "entry-html",
			Content: NewText("<p>html content</p>", "html"),
			Updated: Time(updated),
		}).
		Add(&Entry{
			Title:   "default entry",
			ID:      "entry-default",
			Content: NewText("default content", ""),
			Updated: Time(updated),
		})

	raw, err := feed.Bytes()
	is.Err(t, err, nil)

	var parsed Feed
	is.Err(t, xml.Unmarshal(raw, &parsed), nil)
	is.Equal(t, 3, len(parsed.Entry))

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
		entry := parsed.Entry[i]
		if entry.Content == nil {
			t.Fatalf("expected content element in entry %d", i)
		}
		is.Equal(t, tc.name, entry.Title)
		is.Equal(t, tc.expectedText, entry.Content.Body)
		is.Equal(t, tc.expectedType, entry.Content.Type)
	}
}
