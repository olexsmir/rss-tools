package moviefeed

import (
	"encoding/xml"
	"errors"
	"fmt"
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
	searches map[string]tmdbShow
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

func (f fakeEpisodeAPI) SearchShow(query string) (*tmdbShow, error) {
	if f.searches == nil {
		return nil, fmt.Errorf("no search results for %q", query)
	}
	s, ok := f.searches[query]
	if !ok {
		return nil, fmt.Errorf("no search results for %q", query)
	}
	return &s, nil
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
		shows:     []string{"tt123"},
		nameCache: map[string]string{},
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
		shows:     []string{"bad-show", "tt123"},
		nameCache: map[string]string{},
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

func TestIsDirectID(t *testing.T) {
	is.Equal(t, true, isDirectID("tt1190634"))
	is.Equal(t, true, isDirectID("101"))
	is.Equal(t, true, isDirectID("1"))
	is.Equal(t, false, isDirectID(""))
	is.Equal(t, false, isDirectID("The Boys"))
	is.Equal(t, false, isDirectID("101a"))
}

func TestResolveShowIDDirectPassthrough(t *testing.T) {
	mf := &moviefeed{nameCache: map[string]string{}, api: fakeEpisodeAPI{}}
	id, err := mf.resolveShowID("tt123")
	is.Err(t, err, nil)
	is.Equal(t, "tt123", id)

	id, err = mf.resolveShowID("42")
	is.Err(t, err, nil)
	is.Equal(t, "42", id)
}

func TestResolveShowIDNameIDSkipsSearch(t *testing.T) {
	mf := &moviefeed{nameCache: map[string]string{}, api: fakeEpisodeAPI{}}
	id, err := mf.resolveShowID("The Boys::101")
	is.Err(t, err, nil)
	is.Equal(t, "101", id)
}

func TestResolveShowIDNameIDSkipsSearchWithIMDB(t *testing.T) {
	mf := &moviefeed{nameCache: map[string]string{}, api: fakeEpisodeAPI{}}
	id, err := mf.resolveShowID("The Boys::tt1190634")
	is.Err(t, err, nil)
	is.Equal(t, "tt1190634", id)
}

func TestResolveShowIDSearchesAndCaches(t *testing.T) {
	mf := &moviefeed{
		nameCache: map[string]string{},
		api: fakeEpisodeAPI{
			searches: map[string]tmdbShow{
				"The Boys": {ID: 1398, Name: "The Boys"},
			},
		},
	}

	id, err := mf.resolveShowID("The Boys")
	is.Err(t, err, nil)
	is.Equal(t, "1398", id)

	cached, ok := mf.nameCache["The Boys"]
	is.Equal(t, true, ok)
	is.Equal(t, "1398", cached)
}

func TestResolveShowIDCacheHit(t *testing.T) {
	mf := &moviefeed{
		nameCache: map[string]string{"The Boys": "1398"},
		api:       fakeEpisodeAPI{}, // empty — would fail if search called
	}

	id, err := mf.resolveShowID("The Boys")
	is.Err(t, err, nil)
	is.Equal(t, "1398", id)
}

func TestResolveShowIDSearchFails(t *testing.T) {
	mf := &moviefeed{
		nameCache: map[string]string{},
		api:       fakeEpisodeAPI{}, // no searches registered
	}

	_, err := mf.resolveShowID("Nonexistent Show")
	if err == nil {
		t.Fatal("expected error for unresolvable name")
	}
}

func TestFetchNewEpisodesResolvesNames(t *testing.T) {
	episodes := []TMDBEpisode{
		{
			ID:            1001,
			Name:          "E1",
			AirDate:       time.Now().AddDate(0, 0, -2).Format(dateFormat),
			EpisodeNumber: 1,
			SeasonNumber:  1,
			ShowName:      "The Boys",
			ShowID:        "1398",
		},
	}

	mf := &moviefeed{
		nameCache: map[string]string{},
		api: fakeEpisodeAPI{
			searches: map[string]tmdbShow{
				"The Boys": {ID: 1398, Name: "The Boys"},
			},
			episodes: map[string][]TMDBEpisode{
				"1398": episodes,
			},
		},
		shows: []string{"The Boys"},
	}

	got, err := mf.fetchNewEpisodes()
	is.Err(t, err, nil)
	is.Equal(t, 1, len(got))
	is.Equal(t, 1001, got[0].ID)
}

func TestFetchNewEpisodesNameIDFormat(t *testing.T) {
	episodes := []TMDBEpisode{
		{
			ID:            1001,
			Name:          "E1",
			AirDate:       time.Now().AddDate(0, 0, -2).Format(dateFormat),
			EpisodeNumber: 1,
			SeasonNumber:  1,
			ShowName:      "The Boys",
			ShowID:        "1398",
		},
	}

	mf := &moviefeed{
		nameCache: map[string]string{},
		api: fakeEpisodeAPI{
			episodes: map[string][]TMDBEpisode{
				"1398": episodes,
			},
		},
		shows: []string{"The Boys::1398"},
	}

	got, err := mf.fetchNewEpisodes()
	is.Err(t, err, nil)
	is.Equal(t, 1, len(got))
	is.Equal(t, 1001, got[0].ID)
}

func TestFetchNewEpisodesContinuesOnResolveError(t *testing.T) {
	mf := &moviefeed{
		nameCache: map[string]string{},
		api: fakeEpisodeAPI{
			episodes: map[string][]TMDBEpisode{},
		},
		shows: []string{"Does Not Exist", "tt1190634"},
	}

	got, err := mf.fetchNewEpisodes()
	is.Err(t, err, nil)
	is.Equal(t, 0, len(got))
}
