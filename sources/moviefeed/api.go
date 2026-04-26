package moviefeed

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	dateFormat       = "2006-01-02"
	tmdbBaseURL      = "https://api.themoviedb.org/3"
	tmdbImageBaseURL = "https://image.tmdb.org/t/p/w500"
)

type tmdbShow struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	FirstAirDate string `json:"first_air_date"`
}

type tmdbShowDetails struct {
	tmdbShow
	NumberOfSeasons int `json:"number_of_seasons"`
}

type TMDBEpisode struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"`
	EpisodeNumber int    `json:"episode_number"`
	SeasonNumber  int    `json:"season_number"`
	StillPath     string `json:"still_path"`
	ShowName      string
	ShowID        string
}

type tmdbFindResponse struct {
	TvResults []tmdbShow `json:"tv_results"`
}

type tmdbSeasonResponse struct {
	Episodes []TMDBEpisode `json:"episodes"`
}

type TMDBAPI struct {
	apiKey string
	client *http.Client
}

func NewTMDBAPI(apiKey string, client *http.Client) *TMDBAPI {
	return &TMDBAPI{
		apiKey: apiKey,
		client: client,
	}
}

func (a *TMDBAPI) FetchEpisodesForShow(showID string) ([]TMDBEpisode, error) {
	tmdbID, err := a.getTMDBID(showID)
	if err != nil {
		return nil, err
	}

	show, err := makeRequest[tmdbShowDetails](a, "/tv/%s", tmdbID)
	if err != nil {
		return nil, err
	}

	if show.NumberOfSeasons == 0 {
		return []TMDBEpisode{}, nil
	}

	seasonData, err := makeRequest[tmdbSeasonResponse](a, "/tv/%s/season/%d", tmdbID, show.NumberOfSeasons)
	if err != nil {
		return nil, err
	}

	var allEpisodes []TMDBEpisode
	for _, ep := range seasonData.Episodes {
		ep.ShowName = show.Name
		ep.ShowID = tmdbID
		allEpisodes = append(allEpisodes, ep)
	}

	return filterRecentEpisodes(allEpisodes), nil
}

func (a *TMDBAPI) getTMDBID(showID string) (string, error) {
	if strings.HasPrefix(showID, "tt") {
		result, err := makeRequest[tmdbFindResponse](a, "/find/%s?external_source=imdb_id", showID)
		if err != nil {
			return "", err
		}

		if len(result.TvResults) == 0 {
			return "", fmt.Errorf("no TMDB show found for IMDB ID %s", showID)
		}

		return fmt.Sprintf("%d", result.TvResults[0].ID), nil
	}
	return showID, nil
}

func makeRequest[T any](a *TMDBAPI, endpoint string, args ...interface{}) (*T, error) {
	u, err := url.Parse(fmt.Sprintf(tmdbBaseURL+endpoint, args...))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	q := u.Query()
	q.Set("api_key", a.apiKey)
	u.RawQuery = q.Encode()

	resp, err := a.client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TMDB API error: %s (status: %d)", string(body), resp.StatusCode)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func filterRecentEpisodes(episodes []TMDBEpisode) []TMDBEpisode {
	var recent []TMDBEpisode
	now := time.Now()
	cutoff := now.AddDate(0, 0, -30)

	for _, ep := range episodes {
		if ep.AirDate == "" {
			continue
		}

		airDate, err := time.Parse(dateFormat, ep.AirDate)
		if err != nil {
			continue
		}

		if airDate.Before(now) && airDate.After(cutoff) {
			recent = append(recent, ep)
		}
	}
	return recent
}
