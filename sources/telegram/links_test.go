package telegram

import (
	"testing"

	"olexsmir.xyz/x/is"
)

func TestLinkifyMessageTextEscapesAndPreservesText(t *testing.T) {
	text := `go <now> https://example.com/page?q=1.`
	html, urls := linkifyMessageText(text)

	is.Equal(t, `go &lt;now&gt; <a href="https://example.com/page?q=1">https://example.com/page?q=1</a>.`, html)
	is.Equal(t, 1, len(urls))
	is.Equal(t, "https://example.com/page?q=1", urls[0])
}

func TestYouTubeCanonicalLink(t *testing.T) {
	canonical, id, ok := youtubeCanonicalLink("https://youtu.be/dQw4w9WgXcQ?t=42")
	is.Equal(t, true, ok)
	is.Equal(t, "dQw4w9WgXcQ", id)
	is.Equal(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", canonical)
}
