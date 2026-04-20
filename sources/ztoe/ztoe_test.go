package ztoe

import (
	"bytes"
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/x/is"
)

func TestParseScheduleFromSampleFixture(t *testing.T) {
	html := mustReadFixture(t, "ztoe-unclean.html")
	schedule, err := parseSchedule(bytes.NewReader(html), "text/html; charset=windows-1251")

	is.Err(t, err, nil)
	is.Equal(t, len(schedule.TimeSlots), 48)
	is.Equal(t, schedule.Date, "10.04.2026")
	is.NotEqual(t, len(schedule.Rows), 0)
	is.NotEqual(t, countOutages(schedule.Rows), 0)
}

func TestParseScheduleFromClenFixture(t *testing.T) {
	html := mustReadFixture(t, "ztoe-clean.html")
	schedule, err := parseSchedule(bytes.NewReader(html), "text/html")

	is.Err(t, err, nil)
	is.Equal(t, len(schedule.TimeSlots), 48)
	is.Equal(t, countOutages(schedule.Rows), 0)
}

func TestHandlerRendersAtomFeedWithOutages(t *testing.T) {
	html := mustReadFixture(t, "ztoe-unclean.html")
	schedule, err := parseSchedule(bytes.NewReader(html), "text/html; charset=windows-1251")
	is.Err(t, err, nil)

	subgroup, ok := subgroupWithOutage(schedule.Rows)
	is.Equal(t, ok, true)

	group, subgroupPart, splitOK := strings.Cut(subgroup, ".")
	is.Equal(t, splitOK, true)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1251")
		_, _ = w.Write(html)
	}))
	defer remote.Close()

	z := ztoe{get: func(ctx context.Context, url string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		is.Err(t, err, nil)
		return remote.Client().Do(req)
	}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ztoe/{group}/{subgroup}", z.handler(remote.URL))

	req := httptest.NewRequest(http.MethodGet, "/ztoe/"+group+"/"+subgroupPart, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, http.StatusOK, rr.Code)
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/atom+xml") {
		t.Fatalf("expected atom response content-type, got %q", got)
	}

	var feed app.AtomFeed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.NotEqual(t, 0, len(feed.Entries))
}

func TestHandlerRendersEmptyAtomFeedForNoOutages(t *testing.T) {
	html := mustReadFixture(t, "ztoe-clean.html")
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(html)
	}))
	defer remote.Close()

	z := ztoe{get: func(ctx context.Context, url string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		is.Err(t, err, nil)
		return remote.Client().Do(req)
	}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ztoe/{group}/{subgroup}", z.handler(remote.URL))

	req := httptest.NewRequest(http.MethodGet, "/ztoe/1/1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var feed app.AtomFeed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.Equal(t, 0, len(feed.Entries))
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	is.Err(t, err, nil)
	return data
}

func countOutages(rows map[string][]bool) int {
	total := 0
	for _, slots := range rows {
		for _, outage := range slots {
			if outage {
				total++
			}
		}
	}
	return total
}

func subgroupWithOutage(rows map[string][]bool) (string, bool) {
	for subgroup, slots := range rows {
		for _, outage := range slots {
			if outage {
				return subgroup, true
			}
		}
	}
	return "", false
}
