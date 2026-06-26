package musicfeed

import (
	"context"
	"encoding/binary"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/rss-tools/app/atom"
)

type artistEntry struct {
	label string
	mbid  string
}

type release struct {
	id             string
	releaseGroupID string
	title          string
	date           time.Time
	releaseType    string
	artistName     string
	label          string
	hasArtwork     bool
	spotifyURL     string
	youtubeURL     string
}

type releaseFetcher interface {
	searchArtist(ctx context.Context, name string) (string, string, error)
	fetchArtist(ctx context.Context, mbid string) (string, error)
	fetchReleases(ctx context.Context, mbid string) ([]mbRelease, error)
}

type musicfeed struct {
	bucket    *app.Bucket
	artists   []string
	api       releaseFetcher
	maxAge    time.Duration
	logger    *slog.Logger
	refreshMu sync.Mutex
	refreshed atomic.Bool
}

func Register(a *app.App) error {
	if len(a.Config.MusicArtists) == 0 {
		return nil
	}

	bucket, err := a.Bucket("musicfeed")
	if err != nil {
		return err
	}

	maxAge := time.Duration(a.Config.MusicMaxAgeDays) * 24 * time.Hour
	if maxAge <= 0 {
		maxAge = 30 * 24 * time.Hour
	}

	mf := &musicfeed{
		bucket:  bucket,
		artists: a.Config.MusicArtists,
		api:     newMusicBrainzAPI(a.Client),
		maxAge:  maxAge,
		logger:  a.Logger,
	}

	a.AddWorker(mf.worker)
	a.Route("GET /music", mf.handleMusic)

	a.Logger.Info("musicfeed source registered")
	return nil
}

func (mf *musicfeed) handleMusic(w http.ResponseWriter, r *http.Request) {
	if !mf.refreshed.Load() && (time.Now().Weekday() == time.Friday || mf.cacheMissing()) {
		mf.refreshMu.Lock()
		if !mf.refreshed.Load() {
			mf.refresh(r.Context())
			mf.refreshed.Store(true)
		}
		mf.refreshMu.Unlock()
	}

	cached, err := mf.bucket.Get([]byte("feed"))
	if err != nil {
		slog.Error("failed to read cached feed", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if cached == nil {
		http.Error(w, "feed not yet available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write(cached)
}

func (mf *musicfeed) worker(ctx context.Context) error {
	mf.logger.Info("starting musicfeed worker")

	// Only refresh on Fridays — releases drop on Friday, so we fetch once weekly
	// to avoid rate-limiting MusicBrainz.
	if time.Now().Weekday() == time.Friday {
		mf.maybeRefresh(ctx)
	}

	for {
		next := nextFridayRefresh(time.Now())
		dur := time.Until(next)
		mf.logger.Info("next music feed refresh", "at", next.Format("2006-01-02 15:04"), "in", dur.Round(time.Second))

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(dur):
			mf.maybeRefresh(ctx)
		}
	}
}

func nextFridayRefresh(after time.Time) time.Time {
	const targetHour = 19
	y, m, d := after.Date()
	loc := after.Location()

	// If today is Friday before target hour, return today at target hour.
	if after.Weekday() == time.Friday {
		target := time.Date(y, m, d, targetHour, 0, 0, 0, loc)
		if after.Before(target) {
			return target
		}
	}

	// Otherwise advance to next Friday at target hour.
	next := time.Date(y, m, d, 0, 0, 0, 0, loc).Add(24 * time.Hour)
	for next.Weekday() != time.Friday {
		next = next.Add(24 * time.Hour)
	}
	return time.Date(next.Year(), next.Month(), next.Day(), targetHour, 0, 0, 0, loc)
}

func (mf *musicfeed) cacheMissing() bool {
	_, err := mf.bucket.Get([]byte("feed"))
	return err != nil
}

func (mf *musicfeed) maybeRefresh(ctx context.Context) {
	mf.refreshMu.Lock()
	defer mf.refreshMu.Unlock()

	if mf.refreshed.Load() {
		raw, err := mf.bucket.Get([]byte("refreshed_at"))
		if err == nil && raw != nil {
			lastRefresh := time.Unix(int64(binary.BigEndian.Uint64(raw)), 0)
			if isSameDay(lastRefresh, time.Now()) {
				return
			}
		}
	}

	mf.logger.Info("starting music feed refresh")
	mf.refresh(ctx)
	mf.refreshed.Store(true)
}

func isSameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (mf *musicfeed) refresh(ctx context.Context) {
	type artistResult struct {
		releases []release
	}

	var mu sync.Mutex
	var all []release
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, raw := range mf.artists {
		wg.Add(1)
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			entry := parseArtistEntry(raw)
			mbid, label := mf.resolveArtist(ctx, entry)
			if mbid == "" {
				mf.logger.Warn("could not resolve artist, skipping", "entry", raw)
				return
			}

			mbReleases, err := mf.api.fetchReleases(ctx, mbid)
			if err != nil {
				mf.logger.Warn("failed to fetch releases", "artist", label, "err", err)
				return
			}

			var artistReleases []release
			for _, r := range mbReleases {
				if r.Date == "" {
					continue
				}
				date := parseMBDate(r.Date)
				if date.IsZero() {
					continue
				}
				if time.Since(date) > mf.maxAge || date.After(time.Now()) {
					continue
				}
				artistName := ""
				if len(r.ArtistCredit) > 0 {
					artistName = r.ArtistCredit[0].Name
				}
				spotifyURL, youtubeURL := extractExtURLs(r.Relations)
				artistReleases = append(artistReleases, release{
					id:             r.ID,
					releaseGroupID: r.ReleaseGroup.ID,
					title:          r.Title,
					date:           date,
					releaseType:    r.ReleaseGroup.PrimaryType,
					artistName:     artistName,
					label:          label,
					hasArtwork:     r.CoverArtArchive.Artwork,
					spotifyURL:     spotifyURL,
					youtubeURL:     youtubeURL,
				})
			}

			mu.Lock()
			all = append(all, artistReleases...)
			mu.Unlock()
		}()
	}

	wg.Wait()

	all = dedupeByReleaseGroup(all)

	sort.Slice(all, func(i, j int) bool {
		return all[i].date.After(all[j].date)
	})

	feed := generateFeed(all)
	bytes, err := feed.Bytes()
	if err != nil {
		mf.logger.Error("failed to serialize feed", "err", err)
		return
	}

	if err := mf.bucket.Set([]byte("feed"), bytes); err != nil {
		mf.logger.Error("failed to cache feed", "err", err)
	}

	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(time.Now().Unix()))
	if err := mf.bucket.Set([]byte("refreshed_at"), ts[:]); err != nil {
		mf.logger.Error("failed to save refresh timestamp", "err", err)
	}

	mf.logger.Info("music feed refreshed", "releases", len(all))
}

func (mf *musicfeed) resolveArtist(ctx context.Context, entry artistEntry) (string, string) {
	if entry.mbid != "" {
		return entry.mbid, entry.label
	}

	cached, err := mf.bucket.Get([]byte("mapping:" + entry.label))
	if err == nil && cached != nil {
		return string(cached), entry.label
	}

	if isMBID(entry.label) {
		name, ferr := mf.api.fetchArtist(ctx, entry.label)
		if ferr != nil {
			mf.logger.Warn("failed to fetch artist name", "mbid", entry.label, "err", ferr)
			return entry.label, entry.label
		}
		if berr := mf.bucket.Set([]byte("mapping:"+name), []byte(entry.label)); berr != nil {
			mf.logger.Warn("failed to cache artist mapping", "err", berr)
		}
		return entry.label, name
	}

	mbid, name, err := mf.api.searchArtist(ctx, entry.label)
	if err != nil {
		mf.logger.Warn("failed to search artist", "label", entry.label, "err", err)
		return "", entry.label
	}

	if err := mf.bucket.Set([]byte("mapping:"+entry.label), []byte(mbid)); err != nil {
		mf.logger.Warn("failed to cache artist mapping", "err", err)
	}

	return mbid, name
}

func parseArtistEntry(raw string) artistEntry {
	label, mbid, found := strings.Cut(raw, "::")
	if found {
		return artistEntry{label: strings.TrimSpace(label), mbid: strings.TrimSpace(mbid)}
	}
	return artistEntry{label: strings.TrimSpace(raw)}
}

func isMBID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

func dedupeByReleaseGroup(releases []release) []release {
	seen := make(map[string]int)
	var out []release
	for _, r := range releases {
		if r.releaseGroupID == "" {
			out = append(out, r)
			continue
		}
		if idx, ok := seen[r.releaseGroupID]; ok {
			if r.hasArtwork && !out[idx].hasArtwork {
				out[idx] = r
			}
			continue
		}
		seen[r.releaseGroupID] = len(out)
		out = append(out, r)
	}
	return out
}

func parseMBDate(s string) time.Time {
	formats := []string{"2006-01-02", "2006-01", "2006"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func extractExtURLs(rels []mbRelation) (spotifyURL, youtubeURL string) {
	for _, rel := range rels {
		if rel.Direction != "forward" {
			continue
		}
		resource := rel.URL.Resource
		switch {
		case strings.Contains(resource, "open.spotify.com"):
			spotifyURL = resource
		case strings.Contains(resource, "youtube.com") || strings.Contains(resource, "youtu.be"):
			youtubeURL = resource
		}
	}
	return
}

func generateFeed(releases []release) *atom.Feed {
	feed := atom.NewFeed("New Music Releases", "musicfeed")
	for _, r := range releases {
		displayName := r.label
		if displayName == "" {
			displayName = r.artistName
		}

		links := []atom.Link{{
			Rel:  "alternate",
			Href: fmt.Sprintf("https://musicbrainz.org/release/%s", r.id),
		}}

		content, contentType := releaseContent(r, displayName)

		if r.hasArtwork {
			links = append(links, atom.Link{
				Rel:  "enclosure",
				Type: "image/jpeg",
				Href: fmt.Sprintf("%s/release/%s/front-250.jpg", caaBaseURL, r.id),
			})
		}

		if r.spotifyURL != "" {
			links = append(links, atom.Link{
				Rel:   "related",
				Href:  r.spotifyURL,
				Title: "Spotify",
			})
		}
		if r.youtubeURL != "" {
			links = append(links, atom.Link{
				Rel:   "related",
				Href:  r.youtubeURL,
				Title: "YouTube",
			})
		}

		releaseType := strings.TrimSpace(r.releaseType)
		title := fmt.Sprintf("%s — %s", displayName, r.title)
		if releaseType != "" {
			title += fmt.Sprintf(" (%s)", releaseType)
		}

		feed.Add(&atom.Entry{
			ID:      r.id,
			Title:   title,
			Content: atom.NewText(content, contentType),
			Updated: atom.Time(r.date),
			Link:    links,
		})
	}
	return feed
}

func releaseContent(r release, displayName string) (string, string) {
	if !r.hasArtwork {
		releaseType := strings.TrimSpace(r.releaseType)
		if releaseType != "" {
			return fmt.Sprintf("%s by %s (%s)", r.title, displayName, releaseType), ""
		}
		return fmt.Sprintf("%s by %s", r.title, displayName), ""
	}

	imageURL := fmt.Sprintf("%s/release/%s/front-250.jpg", caaBaseURL, r.id)
	parts := make([]string, 0, 6)
	parts = append(parts, "<body>")

	releaseType := strings.TrimSpace(r.releaseType)
	var text string
	if releaseType != "" {
		text = fmt.Sprintf("%s by %s (%s)", r.title, displayName, releaseType)
	} else {
		text = fmt.Sprintf("%s by %s", r.title, displayName)
	}
	parts = append(parts, "<p>"+html.EscapeString(text)+"</p>")
	parts = append(parts,
		fmt.Sprintf(`<p><img src="%s" alt="%s"/></p>`, html.EscapeString(imageURL), html.EscapeString(r.title)))

	if r.spotifyURL != "" {
		parts = append(parts, fmt.Sprintf(`<p><a href="%s">Spotify</a></p>`, html.EscapeString(r.spotifyURL)))
	}
	if r.youtubeURL != "" {
		parts = append(parts, fmt.Sprintf(`<p><a href="%s">YouTube</a></p>`, html.EscapeString(r.youtubeURL)))
	}

	parts = append(parts, "</body>")

	return strings.Join(parts, ""), "xhtml"
}
