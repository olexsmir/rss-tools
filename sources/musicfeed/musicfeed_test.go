package musicfeed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.etcd.io/bbolt"
	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/x/is"
)

type fakeMusicAPI struct {
	artists  map[string]string
	releases map[string][]mbRelease
	errs     map[string]error
}

func (f fakeMusicAPI) searchArtist(_ context.Context, name string) (string, string, error) {
	if err, ok := f.errs[name]; ok {
		return "", "", err
	}
	mbid, ok := f.artists[name]
	if !ok {
		return "", "", nil
	}
	return mbid, name, nil
}

func (f fakeMusicAPI) fetchArtist(_ context.Context, mbid string) (string, error) {
	for name, id := range f.artists {
		if id == mbid {
			return name, nil
		}
	}
	return "", nil
}

func (f fakeMusicAPI) fetchReleases(_ context.Context, mbid string) ([]mbRelease, error) {
	if err, ok := f.errs[mbid]; ok {
		return nil, err
	}
	return f.releases[mbid], nil
}

func TestParseArtistEntryLabelMbid(t *testing.T) {
	entry := parseArtistEntry("orphan::2a9e4c32-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
	is.Equal(t, "orphan", entry.label)
	is.Equal(t, "2a9e4c32-xxxx-xxxx-xxxx-xxxxxxxxxxxx", entry.mbid)
}

func TestParseArtistEntryPlainSlug(t *testing.T) {
	entry := parseArtistEntry("metallica")
	is.Equal(t, "metallica", entry.label)
	is.Equal(t, "", entry.mbid)
}

func TestParseArtistEntryRawMBID(t *testing.T) {
	entry := parseArtistEntry("2a9e4c32-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
	is.Equal(t, "2a9e4c32-xxxx-xxxx-xxxx-xxxxxxxxxxxx", entry.label)
	is.Equal(t, "", entry.mbid)
}

func TestParseArtistEntryWhitespace(t *testing.T) {
	entry := parseArtistEntry("  orphan::  2a9e4c32-xxxx  ")
	is.Equal(t, "orphan", entry.label)
	is.Equal(t, "2a9e4c32-xxxx", entry.mbid)
}

func TestIsMBIDValid(t *testing.T) {
	is.Equal(t, true, isMBID("2a9e4c32-abcd-4ef8-9abc-123456789abc"))
	is.Equal(t, true, isMBID("550e8400-e29b-41d4-a716-446655440000"))
}

func TestIsMBIDInvalid(t *testing.T) {
	is.Equal(t, false, isMBID(""))
	is.Equal(t, false, isMBID("metallica"))
	is.Equal(t, false, isMBID("550e8400-e29b-41d4-a716-44665544000"))   // 35 chars
	is.Equal(t, false, isMBID("550e8400-e29b-41d4-a716-4466554400000")) // 37 chars
	is.Equal(t, false, isMBID("550e8400:e29b-41d4-a716-446655440000"))  // wrong separator
}

func TestParseMBDateFull(t *testing.T) {
	d := parseMBDate("2026-04-22")
	is.Equal(t, 2026, d.Year())
	is.Equal(t, time.Month(4), d.Month())
	is.Equal(t, 22, d.Day())
}

func TestParseMBDateYearMonth(t *testing.T) {
	d := parseMBDate("2026-04")
	is.Equal(t, 2026, d.Year())
	is.Equal(t, time.Month(4), d.Month())
	is.Equal(t, 1, d.Day())
}

func TestParseMBDateYearOnly(t *testing.T) {
	d := parseMBDate("2026")
	is.Equal(t, 2026, d.Year())
	is.Equal(t, time.Month(1), d.Month())
	is.Equal(t, 1, d.Day())
}

func TestParseMBDateInvalid(t *testing.T) {
	is.Equal(t, true, parseMBDate("").IsZero())
	is.Equal(t, true, parseMBDate("not-a-date").IsZero())
}

func TestDedupeByReleaseGroup(t *testing.T) {
	releases := []release{
		{id: "r1", releaseGroupID: "g1", title: "Album CD", hasArtwork: false},
		{id: "r2", releaseGroupID: "g1", title: "Album Vinyl", hasArtwork: true},
		{id: "r3", releaseGroupID: "g2", title: "Single A", hasArtwork: false},
	}

	deduped := dedupeByReleaseGroup(releases)
	is.Equal(t, 2, len(deduped))
	is.Equal(t, "Album Vinyl", deduped[0].title)
	is.Equal(t, "Single A", deduped[1].title)
	is.Equal(t, true, deduped[0].hasArtwork)
}

func TestDedupeByReleaseGroupNoID(t *testing.T) {
	releases := []release{
		{id: "r1", title: "No group", releaseGroupID: ""},
	}
	deduped := dedupeByReleaseGroup(releases)
	is.Equal(t, 1, len(deduped))
}

func TestReleaseContentWithoutArtwork(t *testing.T) {
	r := release{
		title:       "Porcelain",
		releaseType: "Album",
		hasArtwork:  false,
	}
	content, ctype := releaseContent(r, "Orphan")
	is.Equal(t, "", ctype)
	is.Equal(t, "Porcelain by Orphan (Album)", content)
}

func TestReleaseContentWithoutArtworkAndType(t *testing.T) {
	r := release{
		title:       "Porcelain",
		releaseType: "",
		hasArtwork:  false,
	}
	content, ctype := releaseContent(r, "Orphan")
	is.Equal(t, "", ctype)
	is.Equal(t, "Porcelain by Orphan", content)
}

func TestReleaseContentWithArtwork(t *testing.T) {
	r := release{
		id:          "mbid-123",
		title:       "Porcelain",
		releaseType: "Album",
		hasArtwork:  true,
	}
	content, ctype := releaseContent(r, "Orphan")
	is.Equal(t, "xhtml", ctype)
	is.Equal(t, strings.Contains(content, "<body>"), true)
	is.Equal(t, strings.Contains(content, "Porcelain by Orphan (Album)"), true)
	is.Equal(t, strings.Contains(content, `<img src="https://coverartarchive.org/release/mbid-123/front-250.jpg"`), true)
	is.Equal(t, strings.Contains(content, "</body>"), true)
}

func TestGenerateFeed(t *testing.T) {
	now := time.Now()
	releases := []release{
		{
			id:          "mbid-1",
			title:       "New Album",
			date:        now.Add(-24 * time.Hour),
			releaseType: "Album",
			artistName:  "Test Band",
			label:       "test-band",
			hasArtwork:  false,
		},
		{
			id:          "mbid-2",
			title:       "New Single",
			date:        now.Add(-48 * time.Hour),
			releaseType: "Single",
			artistName:  "Test Band",
			label:       "",
			hasArtwork:  false,
		},
	}

	feed := generateFeed(releases)
	is.Equal(t, "New Music Releases", feed.Title)
	is.Equal(t, "musicfeed", feed.ID)
	is.Equal(t, 2, len(feed.Entry))

	is.Equal(t, "test-band — New Album (Album)", feed.Entry[0].Title)
	is.Equal(t, "mbid-1", feed.Entry[0].ID)
	is.Equal(t, "Test Band — New Single (Single)", feed.Entry[1].Title)
}

func TestGenerateFeedWithLabelFallback(t *testing.T) {
	r := []release{{
		id: "mbid-1", title: "Album", date: time.Now(),
		releaseType: "Album", artistName: "Real Band Name", label: "",
	}}
	feed := generateFeed(r)
	is.Equal(t, "Real Band Name — Album (Album)", feed.Entry[0].Title)
}

func TestGenerateFeedWithArtworkLink(t *testing.T) {
	r := []release{{
		id: "mbid-1", title: "Album", date: time.Now(),
		releaseType: "Album", artistName: "Band", label: "band",
		hasArtwork: true,
	}}
	feed := generateFeed(r)
	is.Equal(t, 2, len(feed.Entry[0].Link))
	is.Equal(t, "alternate", feed.Entry[0].Link[0].Rel)
	is.Equal(t, "enclosure", feed.Entry[0].Link[1].Rel)
	is.Equal(t, "image/jpeg", feed.Entry[0].Link[1].Type)
}

func TestHandleMusicServesCachedFeed(t *testing.T) {
	bucket := newBucket(t)

	err := bucket.Set([]byte("feed"), []byte("<feed>test</feed>"))
	is.Err(t, err, nil)

	mf := &musicfeed{bucket: bucket}
	mf.refreshed.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /music", mf.handleMusic)

	req := httptest.NewRequest(http.MethodGet, "/music", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusOK)
	is.Equal(t, strings.Contains(rr.Header().Get("Content-Type"), "application/atom+xml"), true)
	is.Equal(t, "<feed>test</feed>", rr.Body.String())
}

func TestHandleMusicNoCache(t *testing.T) {
	bucket := newBucket(t)
	mf := &musicfeed{bucket: bucket}
	mf.refreshed.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /music", mf.handleMusic)

	req := httptest.NewRequest(http.MethodGet, "/music", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusServiceUnavailable)
}

func TestResolveArtistCachedMapping(t *testing.T) {
	bucket := newBucket(t)
	bucket.Set([]byte("mapping:metallica"), []byte("mbid-123"))

	mf := &musicfeed{
		bucket: bucket,
		api: fakeMusicAPI{
			artists: map[string]string{"metallica": "mbid-999"},
		},
	}

	mbid, label := mf.resolveArtist(context.Background(), artistEntry{label: "metallica"})
	is.Equal(t, "mbid-123", mbid)
	is.Equal(t, "metallica", label)
}

func TestResolveArtistWithExplicitMBID(t *testing.T) {
	mf := &musicfeed{
		api: fakeMusicAPI{
			artists: map[string]string{"Orphan": "mbid-orphan"},
		},
	}

	mbid, label := mf.resolveArtist(context.Background(), artistEntry{label: "orphan", mbid: "mbid-orphan"})
	is.Equal(t, "mbid-orphan", mbid)
	is.Equal(t, "orphan", label)
}

func TestIsSameDay(t *testing.T) {
	now := time.Now()
	is.Equal(t, true, isSameDay(now, now))
	is.Equal(t, true, isSameDay(now, now.Add(time.Hour)))
	is.Equal(t, false, isSameDay(now, now.Add(24*time.Hour)))
}

func newBucket(t *testing.T) *app.Bucket {
	t.Helper()
	db, err := bbolt.Open(t.TempDir()+"/test.db", 0o600, nil)
	is.Err(t, err, nil)
	t.Cleanup(func() { db.Close() })

	a := app.New(&app.Config{}, db)
	bucket, err := a.Bucket("musicfeed")
	is.Err(t, err, nil)
	return bucket
}
