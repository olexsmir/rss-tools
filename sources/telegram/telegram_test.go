package telegram

import (
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/x/is"
)

func TestFeedEntryFromMessageWithImage(t *testing.T) {
	msg := &Message{
		MessageID:     42,
		Caption:       "hello <world>",
		Date:          time.Date(2026, 4, 22, 19, 38, 0, 0, time.UTC).Unix(),
		PhotoBase64:   "YWJj",
		PhotoMIMEType: "image/png",
	}

	entry := feedEntryFromMessage(msg)
	is.Equal(t, "🖼️ [2026-04-22]", entry.Title)
	if entry.Content == nil {
		t.Fatalf("expected content in image entry")
	}
	is.Equal(t, "html", entry.Content.Type)
	if !strings.Contains(entry.Content.Body, "<p>hello &lt;world&gt;</p>") {
		t.Fatalf("expected escaped text in image entry: %s", entry.Content.Body)
	}
	if !strings.Contains(entry.Content.Body, `src="data:image/png;base64,YWJj"`) {
		t.Fatalf("expected image data URI in image entry: %s", entry.Content.Body)
	}
}

func TestFeedEntryFromMessageWithMultipleImages(t *testing.T) {
	msg := &Message{
		MessageID: 55,
		Caption:   "multi image",
		Date:      time.Date(2026, 4, 23, 12, 15, 0, 0, time.UTC).Unix(),
		PhotoAttachments: []PhotoAttachment{
			{Base64: "YWJj", MIMEType: "image/png"},
			{Base64: "ZGVm", MIMEType: "image/jpeg"},
		},
	}

	entry := feedEntryFromMessage(msg)
	if entry.Content == nil {
		t.Fatalf("expected content in image entry")
	}
	if !strings.Contains(entry.Content.Body, `src="data:image/png;base64,YWJj"`) {
		t.Fatalf("expected first image data URI in image entry: %s", entry.Content.Body)
	}
	if !strings.Contains(entry.Content.Body, `src="data:image/jpeg;base64,ZGVm"`) {
		t.Fatalf("expected second image data URI in image entry: %s", entry.Content.Body)
	}
}

func TestFeedEntryFromMessageTextOnly(t *testing.T) {
	msg := &Message{
		MessageID: 11,
		Text:      "plain text",
		Date:      time.Date(2026, 4, 22, 19, 38, 0, 0, time.UTC).Unix(),
	}

	entry := feedEntryFromMessage(msg)
	is.Equal(t, "plain text", entry.Title)
	if entry.Content == nil {
		t.Fatalf("expected content in text entry")
	}
	is.Equal(t, "text", entry.Content.Type)
	is.Equal(t, "plain text", entry.Content.Body)
}

func TestFeedEntryFromMessagePreservesNewlines(t *testing.T) {
	msg := &Message{
		MessageID: 12,
		Text:      "line 1\nline 2",
		Date:      time.Date(2026, 4, 22, 19, 38, 0, 0, time.UTC).Unix(),
	}

	entry := feedEntryFromMessage(msg)
	if entry.Content == nil {
		t.Fatalf("expected content in text entry")
	}
	is.Equal(t, "html", entry.Content.Type)
	if !strings.Contains(entry.Content.Body, "line 1<br/>line 2") {
		t.Fatalf("expected line breaks preserved in content: %s", entry.Content.Body)
	}
}

func TestFeedEntryFromMessageLinkifiesAndAddsAtomLinks(t *testing.T) {
	msg := &Message{
		MessageID: 15,
		Text:      "watch https://example.com and https://youtu.be/dQw4w9WgXcQ.",
		Date:      time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC).Unix(),
	}

	entry := feedEntryFromMessage(msg)
	if entry.Content == nil {
		t.Fatalf("expected content in link entry")
	}
	is.Equal(t, "html", entry.Content.Type)
	if !strings.Contains(entry.Content.Body, `<a href="https://example.com">https://example.com</a>`) {
		t.Fatalf("expected generic link in content: %s", entry.Content.Body)
	}
	if !strings.Contains(entry.Content.Body, `<a href="https://youtu.be/dQw4w9WgXcQ">https://youtu.be/dQw4w9WgXcQ</a>`) {
		t.Fatalf("expected youtube link in content: %s", entry.Content.Body)
	}

	is.Equal(t, 2, len(entry.Link))
	is.Equal(t, "https://example.com", entry.Link[0].Href)
	is.Equal(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", entry.Link[1].Href)
	is.Equal(t, "yt:video:dQw4w9WgXcQ", entry.ID)
}

func TestFeedEntryFromMessageUsesStoredLinkTitleForSingleLink(t *testing.T) {
	msg := &Message{
		MessageID: 16,
		Text:      "https://example.com/post",
		Date:      time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC).Unix(),
		LinkTitles: map[string]string{
			"https://example.com/post": "Example Post Title",
		},
	}

	entry := feedEntryFromMessage(msg)
	is.Equal(t, "Example Post Title", entry.Title)
}
