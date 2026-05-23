package musicfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	mbBaseURL   = "https://musicbrainz.org/ws/2"
	caaBaseURL  = "https://coverartarchive.org"
	mbUserAgent = "rss-tools/1.0 ( https://github.com/olexsmir/rss-tools )"
)

type mbRelease struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	ArtistCredit []struct {
		Name string `json:"name"`
	} `json:"artist-credit"`
	ReleaseGroup struct {
		ID          string `json:"id"`
		PrimaryType string `json:"primary-type"`
	} `json:"release-group"`
	CoverArtArchive struct {
		Artwork bool `json:"artwork"`
	} `json:"cover-art-archive"`
}

type mbReleaseResponse struct {
	Releases []mbRelease `json:"releases"`
}

type mbArtistSearchResponse struct {
	Artists []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Disambiguation string `json:"disambiguation,omitempty"`
		Score          int    `json:"score"`
	} `json:"artists"`
}

type mbArtistResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

const mbRequestGap = 200 * time.Millisecond

type musicbrainzAPI struct {
	client  *http.Client
	mu      sync.Mutex
	lastReq time.Time
}

func newMusicBrainzAPI(client *http.Client) *musicbrainzAPI {
	return &musicbrainzAPI{
		client: client,
	}
}

func (a *musicbrainzAPI) throttle(ctx context.Context) error {
	a.mu.Lock()
	if a.lastReq.IsZero() {
		a.lastReq = time.Now()
		a.mu.Unlock()
		return nil
	}

	scheduled := a.lastReq.Add(mbRequestGap)
	a.lastReq = scheduled
	a.mu.Unlock()

	wait := time.Until(scheduled)
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (a *musicbrainzAPI) doRequest(ctx context.Context, urlStr string) (*http.Response, error) {
	if err := a.throttle(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mbUserAgent)

	slog.Info("musicbrainz API request", "url", urlStr)
	return a.client.Do(req)
}

func (a *musicbrainzAPI) searchArtist(ctx context.Context, name string) (string, string, error) {
	q := url.QueryEscape(name)
	u := fmt.Sprintf("%s/artist?query=artist:%s&limit=5&fmt=json", mbBaseURL, q)
	resp, err := a.doRequest(ctx, u)
	if err != nil {
		return "", "", fmt.Errorf("search artist %q: %w", name, err)
	}
	defer resp.Body.Close()

	var result mbArtistSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode artist search: %w", err)
	}

	if len(result.Artists) == 0 {
		return "", "", fmt.Errorf("no artist found for %q", name)
	}

	best := result.Artists[0]
	slog.Info("resolved artist",
		"query", name,
		"match", best.Name,
		"disambiguation", best.Disambiguation,
		"score", best.Score,
		"mbid", best.ID,
	)
	return best.ID, best.Name, nil
}

func (a *musicbrainzAPI) fetchArtist(ctx context.Context, mbid string) (string, error) {
	u := fmt.Sprintf("%s/artist/%s?fmt=json", mbBaseURL, mbid)
	resp, err := a.doRequest(ctx, u)
	if err != nil {
		return "", fmt.Errorf("fetch artist %s: %w", mbid, err)
	}
	defer resp.Body.Close()

	var result mbArtistResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode artist: %w", err)
	}
	return result.Name, nil
}

func (a *musicbrainzAPI) fetchReleases(ctx context.Context, mbid string) ([]mbRelease, error) {
	u := fmt.Sprintf(
		"%s/release?artist=%s&inc=artist-credits+release-groups&limit=100&fmt=json",
		mbBaseURL, mbid,
	)
	resp, err := a.doRequest(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("fetch releases for %s: %w", mbid, err)
	}
	defer resp.Body.Close()

	var result mbReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	return result.Releases, nil
}
