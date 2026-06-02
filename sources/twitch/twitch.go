package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/rss-tools/app/atom"
)

var defaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

const (
	twitchAPIBase     = "https://api.twitch.tv"
	tokenExpiryBuffer = 5 * time.Minute
)

var twitchAuthBase = "https://id.twitch.tv"

type twitchStream struct {
	ID           string `json:"id"`
	UserLogin    string `json:"user_login"`
	UserName     string `json:"user_name"`
	GameName     string `json:"game_name"`
	Title        string `json:"title"`
	ViewerCount  int    `json:"viewer_count"`
	StartedAt    string `json:"started_at"`
	ThumbnailURL string `json:"thumbnail_url"`
}

type twitchStreamsResponse struct {
	Data []twitchStream `json:"data"`
}

type twitchAuthResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type twitchfeed struct {
	clientID     string
	clientSecret string
	baseURL      string
	client       *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func Register(a *app.App) error {
	if a.Config.TwitchClientID == "" || a.Config.TwitchClientSecret == "" {
		a.Logger.Info("twitch: client_id or client_secret not set, skipping")
		return nil
	}

	tf := &twitchfeed{
		clientID:     a.Config.TwitchClientID,
		clientSecret: a.Config.TwitchClientSecret,
		baseURL:      twitchAPIBase,
		client:       defaultHTTPClient,
	}

	a.Route("GET /twitch/__oauth2", tf.handleTokenStatus)
	a.Route("GET /twitch/{name}", tf.handleStreams)

	a.Logger.Info("twitch source registered")
	return nil
}

func (tf *twitchfeed) handleStreams(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || name == "__oauth2" {
		http.Error(w, "missing streamer name", http.StatusBadRequest)
		return
	}

	stream, err := tf.do(r.Context(), name)
	if err != nil {
		slog.Error("twitch: fetch failed", "user", name, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	feed := atom.NewFeed("Twitch: "+name, "twitch:"+name).
		WithLink("alternate", "https://twitch.tv/"+name)
	if stream != nil {
		feed.Add(entryFromStream(stream))
	}
	if err := feed.Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (tf *twitchfeed) handleTokenStatus(w http.ResponseWriter, r *http.Request) {
	tf.mu.Lock()
	tok := tf.token
	exp := tf.tokenExp
	tf.mu.Unlock()

	info := map[string]any{
		"client_id_set":     tf.clientID != "",
		"secret_configured": tf.clientSecret != "",
		"token_cached":      tok != "",
	}
	if tok != "" {
		info["token_length"] = len(tok)
		info["expires_at"] = exp.Format(time.RFC3339)
		info["expired"] = time.Now().After(exp)
		if len(tok) > 8 {
			info["preview"] = tok[:4] + "..." + tok[len(tok)-4:]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (tf *twitchfeed) do(ctx context.Context, login string) (*twitchStream, error) {
	tok, err := tf.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	stream, err := tf.fetchStream(ctx, tok, login)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (tf *twitchfeed) getToken(ctx context.Context) (string, error) {
	tf.mu.Lock()
	hasToken := tf.token != ""
	expired := time.Now().After(tf.tokenExp)
	tf.mu.Unlock()

	if hasToken && !expired {
		return tf.token, nil
	}

	return tf.authenticate(ctx)
}

func (tf *twitchfeed) authenticate(ctx context.Context) (string, error) {
	v := url.Values{
		"client_id":     {tf.clientID},
		"client_secret": {tf.clientSecret},
		"grant_type":    {"client_credentials"},
	}

	resp, err := tf.client.PostForm(twitchAuthBase+"/oauth2/token", v)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token endpoint: %s %s", resp.Status, body)
	}

	var auth twitchAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}

	tf.mu.Lock()
	tf.token = auth.AccessToken
	tf.tokenExp = time.Now().Add(time.Duration(auth.ExpiresIn)*time.Second - tokenExpiryBuffer)
	tf.mu.Unlock()

	slog.Info("twitch: new token obtained", "expires_in", auth.ExpiresIn)
	return auth.AccessToken, nil
}

func (tf *twitchfeed) fetchStream(ctx context.Context, token, login string) (*twitchStream, error) {
	u := tf.baseURL + "/helix/streams?user_login=" + login
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", tf.clientID)

	resp, err := tf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching streams: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		tf.mu.Lock()
		tf.token = ""
		tf.mu.Unlock()
		return nil, fmt.Errorf("twitch API: %s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch API: %s", resp.Status)
	}

	var result twitchStreamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, nil
	}
	return &result.Data[0], nil
}

func entryFromStream(s *twitchStream) *atom.Entry {
	title := fmt.Sprintf("%s — %s", s.UserName, s.Title)
	if len(title) > 80 {
		title = title[:80] + "…"
	}

	thumbnail := strings.NewReplacer("{width}", "640", "{height}", "360").Replace(s.ThumbnailURL)

	startedAt, _ := time.Parse(time.RFC3339, s.StartedAt)

	content := fmt.Sprintf(
		"<p>Playing <b>%s</b> · %d viewers</p>",
		html.EscapeString(s.GameName), s.ViewerCount,
	)
	if thumbnail != "" {
		content += fmt.Sprintf(`<p><img src="%s" alt="%s stream"/></p>`,
			html.EscapeString(thumbnail), html.EscapeString(s.UserName))
	}

	login := s.UserLogin
	if login == "" {
		login = s.UserName
	}

	return &atom.Entry{
		ID:      "twitch-stream-" + s.ID,
		Title:   title,
		Content: atom.NewText(content, "html"),
		Updated: atom.Time(startedAt),
		Link: []atom.Link{
			{Rel: "alternate", Href: "https://twitch.tv/" + login},
		},
	}
}
