package telegram

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"olexsmir.xyz/rss-tools/app"
)

var (
	linkRe          = regexp.MustCompile(`https?://[^\s<>"']+`)
	youtubeIDRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	trailingPunctRe = regexp.MustCompile(`[.,!?:;)]+$`)
)

type foundLink struct {
	start int
	end   int
	raw   string
}

func findLinks(text string) []foundLink {
	indexes := linkRe.FindAllStringIndex(text, -1)
	links := make([]foundLink, 0, len(indexes))
	for _, idx := range indexes {
		start, end := idx[0], idx[1]
		candidate := text[start:end]
		trimmed := trailingPunctRe.ReplaceAllString(candidate, "")
		if trimmed == "" {
			continue
		}
		trimmedEnd := start + len(trimmed)
		if !isHTTPURL(trimmed) {
			continue
		}
		links = append(links, foundLink{
			start: start,
			end:   trimmedEnd,
			raw:   trimmed,
		})
	}
	return links
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func linkifyMessageText(text string) (string, []string) {
	links := findLinks(text)
	if len(links) == 0 {
		return html.EscapeString(text), nil
	}

	var b strings.Builder
	urls := make([]string, 0, len(links))
	last := 0
	for _, l := range links {
		if l.start < last {
			continue
		}
		b.WriteString(html.EscapeString(text[last:l.start]))
		escaped := html.EscapeString(l.raw)
		fmt.Fprintf(&b, `<a href="%s">%s</a>`, escaped, escaped)
		urls = append(urls, l.raw)
		last = l.end
	}
	b.WriteString(html.EscapeString(text[last:]))
	return b.String(), urls
}

func messageLinks(text string) []string {
	links := findLinks(text)
	out := make([]string, 0, len(links))
	seen := make(map[string]struct{}, len(links))
	for _, link := range links {
		if _, ok := seen[link.raw]; ok {
			continue
		}
		seen[link.raw] = struct{}{}
		out = append(out, link.raw)
	}
	return out
}

func feedLinks(urls []string) []app.FeedLink {
	links := make([]app.FeedLink, 0, len(urls))
	for _, u := range urls {
		links = append(links, app.FeedLink{
			Rel:  "alternate",
			Type: "text/html",
			Href: u,
		})
	}
	return links
}

func youtubeCanonicalLink(raw string) (string, string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	videoID := ""
	switch host {
	case "youtube.com", "youtube-nocookie.com":
		path := strings.TrimSuffix(u.Path, "/")
		switch path {
		case "/watch":
			videoID = u.Query().Get("v")
		default:
			if afterShort, okShort := strings.CutPrefix(path, "/shorts/"); okShort {
				videoID = afterShort
			} else if afterLive, okLive := strings.CutPrefix(path, "/live/"); okLive {
				videoID = afterLive
			}
		}
	case "youtu.be":
		videoID = strings.Trim(u.Path, "/")
	default:
		return "", "", false
	}

	if !youtubeIDRe.MatchString(videoID) {
		return "", "", false
	}

	canonical := "https://www.youtube.com/watch?v=" + videoID
	return canonical, videoID, true
}

func normalizeLinks(rawLinks []string) []string {
	out := make([]string, 0, len(rawLinks))
	seen := make(map[string]struct{}, len(rawLinks))
	for _, raw := range rawLinks {
		normalized := raw
		if canonical, _, ok := youtubeCanonicalLink(raw); ok {
			normalized = canonical
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func firstYouTubeVideoID(urls []string) (string, bool) {
	for _, u := range urls {
		_, videoID, ok := youtubeCanonicalLink(u)
		if ok {
			return videoID, true
		}
	}
	return "", false
}
