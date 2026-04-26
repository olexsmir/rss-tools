package moviefeed

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"olexsmir.xyz/rss-tools/app"
)

type moviefeed struct {
	api    *TMDBAPI
	shows  []string
	client *http.Client
}

func Register(a *app.App) error {
	if a.Config.MoviefeedAPIKey == "" {
		return nil
	}

	mf := &moviefeed{
		api:    NewTMDBAPI(a.Config.MoviefeedAPIKey, a.Client),
		shows:  a.Config.MoviefeedShows,
		client: a.Client,
	}

	a.Route("GET /movies/{rest...}", mf.handleMovies)

	a.Logger.Info("moviefeed source registered")
	return nil
}

func (mf *moviefeed) handleMovies(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")

	if rest == "" {
		mf.handler(w, r, mf.shows)
		return
	}

	ids := strings.Split(rest, "/")
	var requestedIDs []string
	for _, id := range ids {
		if id != "" {
			requestedIDs = append(requestedIDs, id)
		}
	}

	if len(requestedIDs) == 0 {
		mf.handler(w, r, mf.shows)
		return
	}

	mf.handler(w, r, requestedIDs)
}

func (mf *moviefeed) handler(w http.ResponseWriter, r *http.Request, requestedIDs []string) {
	if len(requestedIDs) == 0 {
		http.Error(w, "no movie IDs provided", http.StatusBadRequest)
		return
	}

	var allEpisodes []TMDBEpisode
	for _, showID := range requestedIDs {
		episodes, err := mf.api.FetchEpisodesForShow(showID)
		if err != nil {
			continue
		}
		allEpisodes = append(allEpisodes, episodes...)
	}

	sortEpisodes(allEpisodes)

	feedID := fmt.Sprintf("moviefeed-%s", strings.Join(requestedIDs, "-"))
	feedTitle := fmt.Sprintf("Episodes from %s", strings.Join(requestedIDs, ", "))
	feed := app.NewFeed(feedTitle, feedID)

	for _, ep := range allEpisodes {
		entryID := fmt.Sprintf("tmdb-%s-s%de%d", ep.ShowID, ep.SeasonNumber, ep.EpisodeNumber)
		airDate := parseAirDate(ep.AirDate)

		feed.Add(app.FeedEntry{
			Title: fmt.Sprintf("%s S%dE%d: %s",
				ep.ShowName,
				ep.SeasonNumber,
				ep.EpisodeNumber,
				ep.Name),
			ID:      entryID,
			Content: ep.Overview,
			Updated: airDate,
			Links: []app.FeedLink{
				{
					Rel:  "alternate",
					Href: fmt.Sprintf("https://www.themoviedb.org/tv/episode/%d", ep.ID),
				},
			},
		})

		if ep.StillPath != "" {
			feed.Add(app.FeedEntry{
				Title:   fmt.Sprintf("%s (image)", ep.Name),
				ID:      entryID + "-img",
				Content: fmt.Sprintf("![%s](%s%s)", ep.Name, tmdbImageBaseURL, ep.StillPath),
				Updated: airDate,
				Links: []app.FeedLink{
					{
						Rel:  "alternate",
						Href: fmt.Sprintf("https://www.themoviedb.org/tv/episode/%d", ep.ID),
					},
				},
			})
		}
	}

	if err := feed.Render(w); err != nil {
		http.Error(w, "failed to render feed", http.StatusInternalServerError)
		return
	}
}

func parseAirDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Now()
	}
	t, err := time.Parse(dateFormat, dateStr)
	if err != nil {
		return time.Now()
	}
	return t
}

func sortEpisodes(episodes []TMDBEpisode) {
	for i := range episodes {
		for j := i + 1; j < len(episodes); j++ {
			if episodes[j].AirDate > episodes[i].AirDate || (episodes[j].AirDate == episodes[i].AirDate && episodes[j].EpisodeNumber > episodes[i].EpisodeNumber) {
				episodes[i], episodes[j] = episodes[j], episodes[i]
			}
		}
	}
}
