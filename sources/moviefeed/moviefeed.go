package moviefeed

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/rss-tools/app/atom"
)

type moviefeed struct {
	api   *TMDBAPI
	shows []string
}

func Register(a *app.App) error {
	if a.Config.MoviefeedAPIKey == "" {
		return nil
	}

	mf := &moviefeed{
		api:   NewTMDBAPI(a.Config.MoviefeedAPIKey, a.Client),
		shows: a.Config.MoviefeedShows,
	}

	a.Route("GET /movies", mf.handleMovies)
	a.Route("GET /movies/", mf.handleMovies)

	a.Logger.Info("moviefeed source registered")
	return nil
}

func (mf *moviefeed) handleMovies(w http.ResponseWriter, r *http.Request) {
	episodes, err := mf.fetchNewEpisodes()
	if err != nil {
		slog.Error("failed to fetch episodes", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	feed := generateFeed(episodes)
	if err := feed.Render(w); err != nil {
		http.Error(w, "failed to render feed", http.StatusInternalServerError)
	}
}

func (mf *moviefeed) fetchNewEpisodes() ([]TMDBEpisode, error) {
	var allEpisodes []TMDBEpisode
	for _, showID := range mf.shows {
		episodes, err := mf.api.FetchEpisodesForShow(showID)
		if err != nil {
			slog.Warn("failed to fetch episodes for show", "show", showID, "err", err)
			continue
		}
		allEpisodes = append(allEpisodes, episodes...)
	}
	return allEpisodes, nil
}

func generateFeed(episodes []TMDBEpisode) *atom.Feed {
	feed := atom.NewFeed("moviefeed", "moviefeed")

	for i := len(episodes) - 1; i >= 0; i-- {
		ep := episodes[i]
		airDate, _ := time.Parse(dateFormat, ep.AirDate)
		content, contentType := episodeContent(ep)
		links := []atom.Link{
			{
				Rel:  "alternate",
				Href: fmt.Sprintf("https://www.themoviedb.org/tv/episode/%d", ep.ID),
			},
		}
		if ep.StillPath != "" {
			links = append(links, atom.Link{
				Rel:    "enclosure",
				Type:   "image/jpeg",
				Length: 0,
				Href:   tmdbImageBaseURL + ep.StillPath,
			})
		}

		feed.Add(&atom.Entry{
			ID: fmt.Sprintf("%s-%d-%d", ep.ShowID, ep.SeasonNumber, ep.EpisodeNumber),
			Title: fmt.Sprintf(
				"%s S%dE%d: %s",
				ep.ShowName,
				ep.SeasonNumber,
				ep.EpisodeNumber,
				ep.Name,
			),
			Content: atom.NewText(content, contentType),
			Updated: atom.Time(airDate),
			Link:    links,
		})
	}
	return feed
}

func episodeContent(ep TMDBEpisode) (string, string) {
	if ep.StillPath == "" {
		return ep.Overview, ""
	}

	imageURL := tmdbImageBaseURL + ep.StillPath
	parts := make([]string, 0, 4)
	parts = append(parts, "<body>")
	if text := strings.TrimSpace(ep.Overview); text != "" {
		parts = append(parts, "<p>"+html.EscapeString(text)+"</p>")
	}
	parts = append(parts,
		fmt.Sprintf(`<p><img src="%s" alt="%s"/></p>`, html.EscapeString(imageURL), html.EscapeString(ep.Name)))
	parts = append(parts, "</body>")

	return strings.Join(parts, ""), "xhtml"
}
