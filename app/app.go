package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

type App struct {
	mux           *http.ServeMux
	workers       []func(context.Context) error
	shutdowns     []func(context.Context)
	wg            *sync.WaitGroup
	db            *bbolt.DB
	scraperClient *http.Client

	// TODO: cacher, each scrapper should be able to get it's own cacher
	Config *Config
	Client *http.Client
	Logger *slog.Logger
}

func New(cfg *Config, db *bbolt.DB) *App {
	return &App{
		mux:           http.NewServeMux(),
		workers:       nil,
		wg:            &sync.WaitGroup{},
		db:            db,
		scraperClient: &http.Client{Timeout: 10 * time.Second},

		Logger: slog.Default(),
		Client: &http.Client{Timeout: 31 * time.Second},
		Config: cfg,
	}
}

// Route registers a global route. pattern syntax is the same as in [http.ServeMux].HandlerFunc
func (a App) Route(pattern string, handler http.HandlerFunc) {
	a.mux.HandleFunc(pattern, handler)
}

// AddWorker adds background worker
func (a *App) AddWorker(worker func(ctx context.Context) error) {
	a.workers = append(a.workers, worker)
}

// AddShutdown registers a shutdown hook that will be called when the app stops.
// Shutdown hooks are called in reverse order of registration.
func (a *App) AddShutdown(fn func(ctx context.Context)) {
	a.shutdowns = append(a.shutdowns, fn)
}

const (
	defaultScraperUserAgent = "rss-tools/1.0)"
	defaultScraperAccept    = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
)

// Get is intended for scraping sources; API SDK calls should use [App.Client] directly.
func (a *App) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultScraperUserAgent)
	req.Header.Set("Accept", defaultScraperAccept)
	return a.scraperClient.Do(req)
}

// Start starts an app and with all it's registered sources
func (a *App) Start(ctx context.Context) error {
	// workers
	for _, worker := range a.workers {
		a.wg.Add(1)
		go func(w func(context.Context) error) {
			defer a.wg.Done()
			if err := w(ctx); err != nil {
				slog.ErrorContext(ctx, "worker exited with an error", "err", err)
			}
		}(worker)
	}

	// http server
	handler := a.recoverMiddleware(a.mux)
	handler = a.loggingMiddleware(handler)
	if strings.TrimSpace(a.Config.AuthToken) != "" {
		handler = a.authMiddleware(handler)
	}

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.Config.Port), // fixme
		Handler: handler,
	}

	go func() {
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpSrv.Shutdown(shutdownCtx)
		}()
	}()

	slog.Info("starting http server", "port", a.Config.Port)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	a.wg.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, fn := range a.shutdowns {
		fn(shutdownCtx)
	}

	return nil
}
