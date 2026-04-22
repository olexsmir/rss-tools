package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"olexsmir.xyz/x/is"
)

func TestAuthMiddlewareDisabledWithoutToken(t *testing.T) {
	a := &App{Config: &Config{}}
	handler := a.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/telegram", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	is.Equal(t, http.StatusNoContent, w.Code)
}

func TestAuthMiddlewareAllowsQueryToken(t *testing.T) {
	a := &App{Config: &Config{AuthToken: "secret"}}
	handler := a.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/telegram?token=secret", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	is.Equal(t, http.StatusNoContent, w.Code)
}

func TestAuthMiddlewareAllowsAuthorizationHeader(t *testing.T) {
	a := &App{Config: &Config{AuthToken: "secret"}}
	handler := a.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/telegram", nil)
	r.Header.Set("Authorization", "secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	is.Equal(t, http.StatusNoContent, w.Code)
}

func TestAuthMiddlewareRejectsBadToken(t *testing.T) {
	a := &App{Config: &Config{AuthToken: "secret"}}
	handler := a.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/telegram?token=bad", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	is.Equal(t, http.StatusUnauthorized, w.Code)
	if !strings.Contains(w.Body.String(), "unauthorized") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}
}
