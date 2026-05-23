package moviefeed

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/rss-tools/app/atom"
	"olexsmir.xyz/x/is"
)

type fakeEpisodeAPI struct {
	episodes map[string][]TMDBEpisode
	errs     map[string]error
}

func (f fakeEpisodeAPI) FetchEpisodesForShow(showID string) ([]TMDBEpisode, error) {
	if err, ok := f.errs[showID]; ok {
		return nil, err
	}
	if episodes, ok := f.episodes[showID]; ok {
		return episodes, nil
	}
	return nil, nil
}

func TestHandleMoviesRendersFeedFromConfiguredShows(t *testing.T) {
	episodes := []TMDBEpisode{
		{
			ID:            1001,
			Name:          "Episode 1",
			Overview:      "E1",
			AirDate:       "2026-04-20",
			EpisodeNumber: 1,
			SeasonNumber:  1,
			StillPath:     "/e1.jpg",
			ShowName:      "Test Show",
			ShowID:        "101",
		},
		{
			ID:            1002,
			Name:          "Episode 2",
			Overview:      "E2",
			AirDate:       "2026-04-21",
			EpisodeNumber: 2,
			SeasonNumber:  1,
			StillPath:     "",
			ShowName:      "Test Show",
			ShowID:        "101",
		},
	}

	mf := &moviefeed{
		api: fakeEpisodeAPI{
			episodes: map[string][]TMDBEpisode{
				"tt123": episodes,
			},
		},
		shows: []string{"tt123"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /movies", mf.handleMovies)

	req := httptest.NewRequest(http.MethodGet, "/movies", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusOK)
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/atom+xml") {
		t.Fatalf("expected atom response content-type, got %q", got)
	}

	var feed atom.Feed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.Equal(t, feed.Title, "moviefeed")
	is.Equal(t, len(feed.Entry), 2)
	is.Equal(t, strings.Contains(feed.Entry[0].Title, "S1E2"), true)
	is.Equal(t, feed.Entry[0].Content.Type, "text")
	is.Equal(t, len(feed.Entry[1].Link), 2)
	is.Equal(t, feed.Entry[1].Link[1].Rel, "enclosure")
	is.Equal(t, feed.Entry[1].Link[1].Type, "image/jpeg")
	is.Equal(t, feed.Entry[1].Link[1].Length, uint(0))
	is.Equal(t, feed.Entry[1].Link[1].Href, "https://image.tmdb.org/t/p/w500/e1.jpg")
	is.Equal(t, feed.Entry[1].Content.Type, "xhtml")
}

func TestHandleMoviesContinuesWhenOneShowFails(t *testing.T) {
	episodes := []TMDBEpisode{
		{
			ID:            1001,
			Name:          "Episode 1",
			Overview:      "E1",
			AirDate:       "2026-04-20",
			EpisodeNumber: 1,
			SeasonNumber:  1,
			StillPath:     "/e1.jpg",
			ShowName:      "Test Show",
			ShowID:        "101",
		},
		{
			ID:            1002,
			Name:          "Episode 2",
			Overview:      "E2",
			AirDate:       "2026-04-21",
			EpisodeNumber: 2,
			SeasonNumber:  1,
			StillPath:     "",
			ShowName:      "Test Show",
			ShowID:        "101",
		},
	}

	mf := &moviefeed{
		api: fakeEpisodeAPI{
			episodes: map[string][]TMDBEpisode{
				"tt123": episodes,
			},
			errs: map[string]error{
				"bad-show": errors.New("boom"),
			},
		},
		shows: []string{"bad-show", "tt123"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /movies", mf.handleMovies)

	req := httptest.NewRequest(http.MethodGet, "/movies", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, rr.Code, http.StatusOK)

	var feed atom.Feed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.Equal(t, len(feed.Entry), 2)
}

func TestFilterRecentEpisodes(t *testing.T) {
	now := time.Now()
	recent := now.AddDate(0, 0, -5).Format(dateFormat)
	old := now.AddDate(0, 0, -40).Format(dateFormat)

	episodes := filterRecentEpisodes([]TMDBEpisode{
		{AirDate: recent, Name: "recent"},
		{AirDate: old, Name: "old"},
		{AirDate: "", Name: "missing"},
	})

	is.Equal(t, 1, len(episodes))
	is.Equal(t, "recent", episodes[0].Name)
}

func TestEpisodeContentIncludesImageInBody(t *testing.T) {
	content, contentType := episodeContent(TMDBEpisode{
		Name:      "Episode 1",
		Overview:  "E1",
		StillPath: "/e1.jpg",
	})

	is.Equal(t, contentType, "xhtml")
	is.Equal(t, strings.Contains(content, "<body>"), true)
	is.Equal(t, strings.Contains(content, `<img src="https://image.tmdb.org/t/p/w500/e1.jpg" alt="Episode 1"`), true)
	is.Equal(t, strings.Contains(content, "</body>"), true)
}
