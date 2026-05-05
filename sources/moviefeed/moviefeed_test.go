package moviefeed

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/x/is"
)

func TestHandleMoviesRendersFeedForRequestedID(t *testing.T) {
	server, client := newTMDBStub(t)
	defer server.Close()

	mf := &moviefeed{
		api:   NewTMDBAPI("test-key", client),
		shows: nil,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /movies/{rest...}", mf.handleMovies)

	req := httptest.NewRequest(http.MethodGet, "/movies/tt123", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusOK)
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/atom+xml") {
		t.Fatalf("expected atom response content-type, got %q", got)
	}

	var feed app.AtomFeed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.Equal(t, feed.Title, "Episodes from tt123")
	is.Equal(t, feed.ID, "moviefeed-tt123")
	is.Equal(t, len(feed.Entries), 3)                                  // two episodes + one image entry
	is.Equal(t, strings.Contains(feed.Entries[0].Title, "S1E2"), true) // newest first
	is.Equal(t, feed.Entries[2].ID, "tmdb-101-s1e1-img")
}

func TestHandleMoviesUsesConfiguredShowsForEmptyPath(t *testing.T) {
	server, client := newTMDBStub(t)
	defer server.Close()

	mf := &moviefeed{
		api:   NewTMDBAPI("test-key", client),
		shows: []string{"tt123"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /movies/{rest...}", mf.handleMovies)

	req := httptest.NewRequest(http.MethodGet, "/movies/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusOK)

	var feed app.AtomFeed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.Equal(t, feed.ID, "moviefeed-tt123")
}

func TestHandleMoviesReturnsBadRequestWhenNoIDs(t *testing.T) {
	mf := &moviefeed{
		api:   NewTMDBAPI("test-key", http.DefaultClient),
		shows: nil,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /movies/{rest...}", mf.handleMovies)

	req := httptest.NewRequest(http.MethodGet, "/movies/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusBadRequest)
	is.Equal(t, strings.Contains(rr.Body.String(), "no movie IDs provided"), true)
}

func TestFetchEpisodesForShowFiltersRecentAndMapsFields(t *testing.T) {
	server, client := newTMDBStub(t)
	defer server.Close()

	api := NewTMDBAPI("test-key", client)
	episodes, err := api.FetchEpisodesForShow("tt123")
	is.Err(t, err, nil)

	is.Equal(t, len(episodes), 2) // old episode is filtered out
	is.Equal(t, episodes[0].ShowID, "101")
	is.Equal(t, episodes[0].ShowName, "Test Show")
}

func newTMDBStub(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()

	recentDay := time.Now().AddDate(0, 0, -7).Format(dateFormat)
	newestDay := time.Now().AddDate(0, 0, -1).Format(dateFormat)
	oldDay := time.Now().AddDate(0, 0, -45).Format(dateFormat)

	mux := http.NewServeMux()
	mux.HandleFunc("/3/find/tt123", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("external_source"); got != "imdb_id" {
			t.Fatalf("unexpected external_source query: %q", got)
		}
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Fatalf("unexpected api_key query: %q", got)
		}
		_, _ = w.Write([]byte(`{"tv_results":[{"id":101}]}`))
	})

	mux.HandleFunc("/3/tv/101", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Fatalf("unexpected api_key query: %q", got)
		}
		_, _ = w.Write([]byte(`{"id":101,"name":"Test Show","number_of_seasons":1}`))
	})

	mux.HandleFunc("/3/tv/101/season/1", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api_key"); got != "test-key" {
			t.Fatalf("unexpected api_key query: %q", got)
		}
		body := fmt.Sprintf(`{
			"episodes": [
				{"id": 1001, "name": "Episode 1", "overview": "E1", "air_date": %q, "episode_number": 1, "season_number": 1, "still_path": "/e1.jpg"},
				{"id": 1002, "name": "Episode 2", "overview": "E2", "air_date": %q, "episode_number": 2, "season_number": 1, "still_path": ""},
				{"id": 1003, "name": "Episode old", "overview": "old", "air_date": %q, "episode_number": 3, "season_number": 1, "still_path": ""}
			]
		}`, recentDay, newestDay, oldDay)
		_, _ = w.Write([]byte(body))
	})

	server := httptest.NewServer(mux)
	target, err := url.Parse(server.URL)
	is.Err(t, err, nil)

	client := &http.Client{
		Transport: rewriteTransport{target: target},
	}
	return server, client
}

type rewriteTransport struct {
	target *url.URL
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	copiedURL := *clone.URL
	copiedURL.Scheme = t.target.Scheme
	copiedURL.Host = t.target.Host
	clone.URL = &copiedURL
	clone.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}
