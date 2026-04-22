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
	is.Equal(t, "html", entry.ContentType)
	if !strings.Contains(entry.Content, "<p>hello &lt;world&gt;</p>") {
		t.Fatalf("expected escaped text in image entry: %s", entry.Content)
	}
	if !strings.Contains(entry.Content, `src="data:image/png;base64,YWJj"`) {
		t.Fatalf("expected image data URI in image entry: %s", entry.Content)
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
	is.Equal(t, "", entry.ContentType)
	is.Equal(t, "plain text", entry.Content)
}
