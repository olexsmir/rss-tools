package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"olexsmir.xyz/x/is"
)

func TestEnrichMessageWithLinkTitlesStoresFetchedTitle(t *testing.T) {
	calls := 0
	tg := &telegram{
		get: func(_ context.Context, url string) (*http.Response, error) {
			calls++
			is.Equal(t, "https://example.com/post", url)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/html; charset=utf-8"},
				},
				Body: io.NopCloser(strings.NewReader(`<html><head><title> Example Post Title </title></head></html>`)),
			}, nil
		},
		logger: slog.Default(),
	}
	msg := &Message{Text: "https://example.com/post"}

	changed := tg.enrichMessageWithLinkTitles(context.Background(), msg)
	is.Equal(t, true, changed)
	is.Equal(t, 1, calls)
	is.Equal(t, "Example Post Title", msg.LinkTitles["https://example.com/post"])

	changed = tg.enrichMessageWithLinkTitles(context.Background(), msg)
	is.Equal(t, false, changed)
	is.Equal(t, 1, calls)
}

func TestEnrichMessageWithLinkTitlesRefreshesPlaceholderCachedTitle(t *testing.T) {
	calls := 0
	tg := &telegram{
		get: func(_ context.Context, _ string) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/html; charset=utf-8"},
				},
				Body: io.NopCloser(strings.NewReader(`<html><head><title>Real Video Title</title></head></html>`)),
			}, nil
		},
		logger: slog.Default(),
	}
	msg := &Message{
		Text: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		LinkTitles: map[string]string{
			"https://www.youtube.com/watch?v=dQw4w9WgXcQ": " - YouTube ",
		},
	}

	changed := tg.enrichMessageWithLinkTitles(context.Background(), msg)
	is.Equal(t, true, changed)
	is.Equal(t, 1, calls)
	is.Equal(t, "Real Video Title", msg.LinkTitles["https://www.youtube.com/watch?v=dQw4w9WgXcQ"])
}

func TestIsSingleLinkMessage(t *testing.T) {
	is.Equal(t, true, isSingleLinkMessage(" https://example.com/path. "))
	is.Equal(t, false, isSingleLinkMessage("check https://example.com/path"))
}

func TestEnrichMessageWithLinkTitlesIgnoresNonSingleLinkMessages(t *testing.T) {
	calls := 0
	tg := &telegram{
		get: func(_ context.Context, _ string) (*http.Response, error) {
			calls++
			return nil, nil
		},
		logger: slog.Default(),
	}
	msg := &Message{Text: "check this https://example.com/post"}

	changed := tg.enrichMessageWithLinkTitles(context.Background(), msg)
	is.Equal(t, false, changed)
	is.Equal(t, 0, calls)
}

func TestFetchPageTitleFallsBackToMetaTitleForYouTubePlaceholder(t *testing.T) {
	title, err := fetchPageTitle(context.Background(), func(_ context.Context, _ string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			Body: io.NopCloser(strings.NewReader(`<html><head><title> - YouTube </title><meta property="og:title" content="Real Video Title"></head></html>`)),
		}, nil
	}, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	is.Equal(t, "Real Video Title", title)
}

func TestFetchPageTitleRejectsYouTubePlaceholderWithoutMetadata(t *testing.T) {
	_, err := fetchPageTitle(context.Background(), func(_ context.Context, _ string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			Body: io.NopCloser(strings.NewReader(`<html><head><title> - YouTube </title></head></html>`)),
		}, nil
	}, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err == nil {
		t.Fatalf("expected an error for placeholder title")
	}
}
