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
	api       episodeFetcher
	shows     []string
	nameCache map[string]string
}

type episodeFetcher interface {
	FetchEpisodesForShow(showID string) ([]TMDBEpisode, error)
	SearchShow(query string) (*tmdbShow, error)
}

func Register(a *app.App) error {
	if a.Config.MoviefeedAPIKey == "" {
		return nil
	}

	mf := &moviefeed{
		api:       NewTMDBAPI(a.Config.MoviefeedAPIKey, a.Client),
		shows:     a.Config.MoviefeedShows,
		nameCache: map[string]string{},
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
	for _, entry := range mf.shows {
		showID, err := mf.resolveShowID(entry)
		if err != nil {
			slog.Warn("failed to resolve show", "entry", entry, "err", err)
			continue
		}

		episodes, err := mf.api.FetchEpisodesForShow(showID)
		if err != nil {
			slog.Warn("failed to fetch episodes for show", "show", showID, "entry", entry, "err", err)
			continue
		}
		allEpisodes = append(allEpisodes, episodes...)
	}
	return allEpisodes, nil
}

func (mf *moviefeed) resolveShowID(entry string) (string, error) {
	if cached, ok := mf.nameCache[entry]; ok {
		return cached, nil
	}

	label, id, hasSep := strings.Cut(entry, "::")
	if hasSep && id != "" {
		mf.nameCache[entry] = id
		return id, nil
	}

	name := strings.TrimSpace(label)
	if isDirectID(name) {
		return name, nil
	}

	show, err := mf.api.SearchShow(name)
	if err != nil {
		return "", fmt.Errorf("searching %q: %w", name, err)
	}

	tmdbID := fmt.Sprintf("%d", show.ID)
	mf.nameCache[entry] = tmdbID
	slog.Info("resolved show name", "name", name, "tmdb_id", tmdbID, "first_air", show.FirstAirDate)
	return tmdbID, nil
}

func isDirectID(s string) bool {
	if strings.HasPrefix(s, "tt") {
		return true
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
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
