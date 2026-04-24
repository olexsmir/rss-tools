package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

const maxPageBytes = 2 << 20 // 2 MiB

func fetchPageTitle(ctx context.Context, get func(context.Context, string) (*http.Response, error), rawURL string) (string, error) {
	if get == nil {
		return "", fmt.Errorf("missing page getter")
	}

	resp, err := get(ctx, rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	decoded, err := charset.NewReader(io.LimitReader(resp.Body, maxPageBytes), resp.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(decoded)
	if err != nil {
		return "", err
	}

	title := normalizePageTitle(doc.Find("title").First().Text())
	if !isMeaningfulPageTitle(title) {
		title = metaPageTitle(doc)
	}
	if !isMeaningfulPageTitle(title) {
		return "", fmt.Errorf("page title is empty")
	}
	return title, nil
}

func metaPageTitle(doc *goquery.Document) string {
	selectors := []string{
		`meta[property="og:title"]`,
		`meta[name="og:title"]`,
		`meta[property="twitter:title"]`,
		`meta[name="twitter:title"]`,
		`meta[itemprop="name"]`,
	}

	for _, selector := range selectors {
		content, ok := doc.Find(selector).First().Attr("content")
		if !ok {
			continue
		}
		title := normalizePageTitle(content)
		if isMeaningfulPageTitle(title) {
			return title
		}
	}
	return ""
}

func normalizePageTitle(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func isMeaningfulPageTitle(title string) bool {
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "", "- youtube", "youtube":
		return false
	default:
		return true
	}
}
