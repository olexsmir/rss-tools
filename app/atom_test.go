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
