package twitch

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/rss-tools/app/atom"
	"olexsmir.xyz/x/is"
)

func TestRegisterSkipsWhenNotConfigured(t *testing.T) {
	a := app.App{
		Config: &app.Config{},
		Logger: slog.Default(),
	}

	err := Register(&a)
	is.Err(t, err, nil)
}

func TestHandleTokenStatusCached(t *testing.T) {
	tf := &twitchfeed{
		clientID:     "client123",
		clientSecret: "secret",
		token:        "abc12345xyz67890",
		tokenExp:     time.Now().Add(24 * time.Hour),
	}
	req := httptest.NewRequest(http.MethodGet, "/twitch/__oauth2", nil)
	w := httptest.NewRecorder()
	tf.handleTokenStatus(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	is.Equal(t, http.StatusOK, resp.StatusCode)
	is.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var info map[string]any
	is.Err(t, json.Unmarshal(body, &info), nil)
	is.Equal(t, true, info["client_id_set"].(bool))
	is.Equal(t, true, info["secret_configured"].(bool))
	is.Equal(t, true, info["token_cached"].(bool))
	is.Equal(t, float64(16), info["token_length"].(float64))
	is.Equal(t, false, info["expired"].(bool))
	is.Equal(t, "abc1...7890", info["preview"].(string))
}

func TestHandleTokenStatusNotConfigured(t *testing.T) {
	tf := &twitchfeed{clientID: "", clientSecret: ""}
	req := httptest.NewRequest(http.MethodGet, "/twitch/__oauth2", nil)
	w := httptest.NewRecorder()
	tf.handleTokenStatus(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	var info map[string]any
	is.Err(t, json.Unmarshal(body, &info), nil)
	is.Equal(t, false, info["client_id_set"].(bool))
	is.Equal(t, false, info["token_cached"].(bool))
	_, hasPreview := info["preview"]
	is.Equal(t, false, hasPreview)
}

func TestHandleStreamsLive(t *testing.T) {
	twitchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		is.Equal(t, "/helix/streams", r.URL.Path)
		is.Equal(t, "shroud", r.URL.Query().Get("user_login"))
		is.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		is.Equal(t, "test-client", r.Header.Get("Client-Id"))

		resp := twitchStreamsResponse{
			Data: []twitchStream{{
				ID:           "12345",
				UserLogin:    "shroud",
				UserName:     "shroud",
				GameName:     "VALORANT",
				Title:        "ranked valorant",
				ViewerCount:  12345,
				StartedAt:    "2026-05-27T18:00:00Z",
				ThumbnailURL: "https://example.com/{width}x{height}.jpg",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer twitchSrv.Close()

	tf := &twitchfeed{
		clientID: "test-client",
		token:    "test-token",
		tokenExp: time.Now().Add(24 * time.Hour),
		baseURL:  twitchSrv.URL,
		client:   twitchSrv.Client(),
	}
	req := httptest.NewRequest(http.MethodGet, "/twitch/shroud", nil)
	req.SetPathValue("name", "shroud")
	w := httptest.NewRecorder()

	tf.handleStreams(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	is.Equal(t, http.StatusOK, resp.StatusCode)
	is.Equal(t, "application/atom+xml; charset=utf-8", resp.Header.Get("Content-Type"))

	raw := string(body)
	if !strings.Contains(raw, `<title>shroud — ranked valorant</title>`) {
		t.Fatal("missing title in feed")
	}
	if !strings.Contains(raw, `<id>twitch-stream-12345</id>`) {
		t.Fatal("missing stream ID in feed")
	}
	if !strings.Contains(raw, `<link rel="alternate" href="https://twitch.tv/shroud"`) {
		t.Fatal("missing link in feed")
	}
	if !strings.Contains(raw, `&lt;b&gt;VALORANT&lt;/b&gt;`) {
		t.Fatal("missing game name in feed")
	}
	if !strings.Contains(raw, `12345 viewers`) {
		t.Fatal("missing viewer count in feed")
	}
	if !strings.Contains(raw, `640x360.jpg`) {
		t.Fatal("missing thumbnail in feed")
	}
}

func TestHandleStreamsOffline(t *testing.T) {
	twitchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(twitchStreamsResponse{Data: []twitchStream{}})
	}))
	defer twitchSrv.Close()

	tf := &twitchfeed{
		clientID: "test-client",
		token:    "test-token",
		tokenExp: time.Now().Add(24 * time.Hour),
		baseURL:  twitchSrv.URL,
		client:   twitchSrv.Client(),
	}
	req := httptest.NewRequest(http.MethodGet, "/twitch/offlineuser", nil)
	req.SetPathValue("name", "offlineuser")
	w := httptest.NewRecorder()
	tf.handleStreams(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	is.Equal(t, http.StatusOK, resp.StatusCode)
	raw := string(body)
	if !strings.Contains(raw, `<feed`) {
		t.Fatal("expected feed element")
	}
	if !strings.Contains(raw, `<title>Twitch: offlineuser</title>`) {
		t.Fatal("expected feed title")
	}
	if strings.Contains(raw, `<entry>`) {
		t.Fatal("should have no entries for offline user")
	}
}

func TestHandleStreamsMissingName(t *testing.T) {
	tf := &twitchfeed{clientID: "test-client", clientSecret: "secret", token: "tok", tokenExp: time.Now().Add(24 * time.Hour)}
	req := httptest.NewRequest(http.MethodGet, "/twitch/", nil)
	w := httptest.NewRecorder()
	tf.handleStreams(w, req)

	resp := w.Result()
	is.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestEntryFromStreamTruncatesLongTitle(t *testing.T) {
	longTitle := strings.Repeat("x", 100)
	s := &twitchStream{
		ID:        "1",
		UserName:  "longname",
		Title:     longTitle,
		GameName:  "game",
		StartedAt: "2026-01-01T00:00:00Z",
	}
	entry := entryFromStream(s)
	if len(entry.Title) > 83 {
		t.Fatalf("title too long: %d chars", len(entry.Title))
	}
	if !strings.HasSuffix(entry.Title, "…") {
		t.Fatal("title should end with ellipsis")
	}
}

func TestEntryFromStreamFormatsContent(t *testing.T) {
	s := &twitchStream{
		ID:           "1",
		UserLogin:    "testuser",
		UserName:     "testuser",
		Title:        "test stream",
		GameName:     "Just Chatting",
		ViewerCount:  42,
		StartedAt:    "2026-01-01T00:00:00Z",
		ThumbnailURL: "https://example.com/{width}x{height}.jpg",
	}
	entry := entryFromStream(s)
	is.Equal(t, "twitch-stream-1", entry.ID)
	is.Equal(t, "testuser — test stream", entry.Title)
	is.Equal(t, atom.Time(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), entry.Updated)
	is.Equal(t, "html", entry.Content.Type)
	if !strings.Contains(entry.Content.Body, "Just Chatting") {
		t.Fatal("missing game name")
	}
	if !strings.Contains(entry.Content.Body, "42 viewers") {
		t.Fatal("missing viewer count")
	}
	if !strings.Contains(entry.Content.Body, "https://example.com/640x360.jpg") {
		t.Fatal("missing thumbnail")
	}
	is.Equal(t, "https://twitch.tv/testuser", entry.Link[0].Href)
}

func TestFetchStreamWithTokenNotLiveReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(twitchStreamsResponse{Data: []twitchStream{}})
	}))
	defer srv.Close()

	tf := &twitchfeed{
		clientID: "cid",
		baseURL:  srv.URL,
		client:   srv.Client(),
	}
	stream, err := tf.fetchStream(context.Background(), "tok", "nonexistent")
	is.Err(t, err, nil)
	if stream != nil {
		t.Fatal("expected nil stream for offline user")
	}
}

func TestAuthenticateCachesToken(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		is.Equal(t, "/oauth2/token", r.URL.Path)
		is.Equal(t, "POST", r.Method)
		is.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		is.Err(t, err, nil)
		is.Equal(t, "test-client", r.Form.Get("client_id"))
		is.Equal(t, "test-secret", r.Form.Get("client_secret"))
		is.Equal(t, "client_credentials", r.Form.Get("grant_type"))

		json.NewEncoder(w).Encode(twitchAuthResponse{
			AccessToken: "new-access-token",
			ExpiresIn:   3600,
			TokenType:   "bearer",
		})
	}))
	defer authSrv.Close()

	tf := &twitchfeed{
		clientID:     "test-client",
		clientSecret: "test-secret",
		client:       authSrv.Client(),
	}
	tf.baseURL = "http://doesntmatter" // not used for auth

	// swap auth endpoint
	savedBase := twitchAuthBase
	twitchAuthBase = authSrv.URL
	defer func() { twitchAuthBase = savedBase }()

	tok, err := tf.authenticate(context.Background())
	is.Err(t, err, nil)
	is.Equal(t, "new-access-token", tok)

	// token should be cached on struct
	is.Equal(t, "new-access-token", tf.token)
	if time.Now().After(tf.tokenExp) {
		t.Fatal("expiry should be in the future")
	}
}

func TestGetTokenUsesCachedWhenNotExpired(t *testing.T) {
	tf := &twitchfeed{
		token:    "cached-token",
		tokenExp: time.Now().Add(24 * time.Hour),
	}
	tok, err := tf.getToken(context.Background())
	is.Err(t, err, nil)
	is.Equal(t, "cached-token", tok)
}

func TestGetTokenFetchesNewWhenExpired(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(twitchAuthResponse{
			AccessToken: "refreshed-token",
			ExpiresIn:   3600,
			TokenType:   "bearer",
		})
	}))
	defer authSrv.Close()

	tf := &twitchfeed{
		clientID:     "cid",
		clientSecret: "secret",
		client:       authSrv.Client(),
		token:        "stale-token",
		tokenExp:     time.Now().Add(-1 * time.Hour), // expired
	}

	savedBase := twitchAuthBase
	twitchAuthBase = authSrv.URL
	defer func() { twitchAuthBase = savedBase }()

	tok, err := tf.getToken(context.Background())
	is.Err(t, err, nil)
	is.Equal(t, "refreshed-token", tok)
}
