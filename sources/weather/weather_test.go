package weather

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"olexsmir.xyz/rss-tools/app"
	"olexsmir.xyz/x/is"
)

const forecastFixture = `{
  "timezone": "Europe/Kyiv",
  "current": {
    "time": "2026-05-05T07:30",
    "temperature_2m": 17.1,
    "apparent_temperature": 16.3
  },
  "daily": {
    "time": ["2026-05-05"],
    "temperature_2m_min": [13.2],
    "temperature_2m_max": [22.4],
    "weather_code": [3],
    "precipitation_probability_max": [70],
    "precipitation_sum": [9.2],
    "rain_sum": [5.1],
    "showers_sum": [4.1],
    "wind_speed_10m_max": [25.0],
    "wind_direction_10m_dominant": [315]
  },
  "hourly": {
    "time": [
      "2026-05-05T00:00",
      "2026-05-05T04:00",
      "2026-05-05T08:00",
      "2026-05-05T12:00",
      "2026-05-05T16:00",
      "2026-05-05T20:00"
    ],
    "temperature_2m": [15.1, 13.0, 14.1, 18.2, 21.4, 16.3],
    "weather_code": [2, 2, 3, 2, 80, 2],
    "precipitation_probability": [10, 20, 25, 35, 70, 45],
    "wind_speed_10m": [12.0, 14.0, 16.0, 20.0, 25.0, 18.0],
    "relative_humidity_2m": [85, 88, 82, 70, 60, 72]
  }
}`

const airFixture = `{
  "current": {
    "us_aqi": 42,
    "pm2_5": 8.1,
    "pm10": 12.4,
    "ozone": 55.3,
    "nitrogen_dioxide": 7.0
  }
}`

const geocodeFixture = `{
  "display_name": "Kyiv, Kyiv Oblast, Ukraine",
  "address": {
    "city": "Kyiv",
    "state": "Kyiv Oblast",
    "country": "Ukraine"
  }
}`

func TestWeatherHandlerRendersMorningBriefing(t *testing.T) {
	forecastSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(forecastFixture))
	}))
	defer forecastSrv.Close()

	airSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(airFixture))
	}))
	defer airSrv.Close()

	geoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(geocodeFixture))
	}))
	defer geoSrv.Close()

	w := &weather{
		client:        forecastSrv.Client(),
		forecastURL:   forecastSrv.URL,
		airQualityURL: airSrv.URL,
		geocodingURL:  geoSrv.URL,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /weather", w.handler)

	req := httptest.NewRequest(http.MethodGet, "/weather?latitude=50.4501&longitude=30.5234", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, http.StatusOK, rr.Code)
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/atom+xml") {
		t.Fatalf("expected atom feed, got %q", got)
	}

	var feed app.AtomFeed
	is.Err(t, xml.NewDecoder(rr.Body).Decode(&feed), nil)
	is.Equal(t, len(feed.Entries), 1)
	is.Equal(t, feed.Title, "Weather forecast for Kyiv")
	is.Equal(t, feed.Entries[0].Content.Type, "html")
	is.Equal(t, strings.HasPrefix(feed.Entries[0].Content.Value, "<pre>"), true)

	content := feed.Entries[0].Content.Value
	if !strings.Contains(content, "+13° / +22°  |  Cloudy with showers in the afternoon") {
		t.Fatalf("missing summary line in content:\n%s", content)
	}
	if !strings.Contains(content, "☂ 70% chance of rain (5-9 mm)") {
		t.Fatalf("missing rain line in content:\n%s", content)
	}
	if !strings.Contains(content, "🌬 Wind: 12-25 km/h from NW") {
		t.Fatalf("missing wind line in content:\n%s", content)
	}
	if !strings.Contains(content, "🌡 Now: +17° (feels like +16°)") {
		t.Fatalf("missing feels-like line in content:\n%s", content)
	}
	if !strings.Contains(content, "💧 Humidity: 60-88%") {
		t.Fatalf("missing humidity line in content:\n%s", content)
	}
	if !strings.Contains(content, "🌫 Air: AQI 42 (Good), PM2.5 8.1, PM10 12.4, O3 55.3, NO2 7.0") {
		t.Fatalf("missing air quality line in content:\n%s", content)
	}
	for _, line := range []string{
		"08:00  +14°  ☁ Cloudy",
		"12:00  +18°  🌥 Partly cloudy",
		"16:00  +21°  🌧 Rain showers",
		"20:00  +16°  🌥 Partly cloudy",
	} {
		if !strings.Contains(content, line) {
			t.Fatalf("missing timeline line %q in content:\n%s", line, content)
		}
	}
}

func TestWeatherHandlerBadCoordinates(t *testing.T) {
	w := &weather{
		client:        http.DefaultClient,
		forecastURL:   "http://example.test/forecast",
		airQualityURL: "http://example.test/air",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /weather", w.handler)

	for _, route := range []string{
		"/weather",
		"/weather?latitude=abc&longitude=30.5",
		"/weather?latitude=95&longitude=30.5",
		"/weather?latitude=50.5&longitude=200",
	} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		is.Equal(t, http.StatusBadRequest, rr.Code)
	}
}

func TestWeatherHandlerUpstreamFailure(t *testing.T) {
	forecastSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer forecastSrv.Close()

	airSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(airFixture))
	}))
	defer airSrv.Close()

	w := &weather{
		client:        forecastSrv.Client(),
		forecastURL:   forecastSrv.URL,
		airQualityURL: airSrv.URL,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /weather", w.handler)

	req := httptest.NewRequest(http.MethodGet, "/weather?latitude=50.45&longitude=30.52", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	is.Equal(t, http.StatusBadGateway, rr.Code)
}

func TestWindDirectionFromDegrees(t *testing.T) {
	is.Equal(t, windDirectionFromDegrees(0), "N")
	is.Equal(t, windDirectionFromDegrees(45), "NE")
	is.Equal(t, windDirectionFromDegrees(180), "S")
	is.Equal(t, windDirectionFromDegrees(315), "NW")
}

func TestTimelineSummaryUsesRequestedHours(t *testing.T) {
	forecast := forecastResponse{
		Timezone: "Europe/Kyiv",
	}
	forecast.Hourly.Time = []string{
		"2026-05-05T07:00",
		"2026-05-05T11:00",
		"2026-05-05T15:00",
		"2026-05-05T19:00",
	}
	forecast.Hourly.Temperature = []float64{14.2, 18.4, 21.0, 16.7}
	forecast.Hourly.WeatherCode = []int{3, 2, 80, 2}

	loc, err := timeLoadLocation("Europe/Kyiv")
	is.Err(t, err, nil)
	lines := timelineSummary(forecast, "2026-05-05", loc)

	is.Equal(t, len(lines), 4)
	is.Equal(t, strings.HasPrefix(lines[0], "08:00"), true)
	is.Equal(t, strings.HasPrefix(lines[1], "12:00"), true)
	is.Equal(t, strings.HasPrefix(lines[2], "16:00"), true)
	is.Equal(t, strings.HasPrefix(lines[3], "20:00"), true)
}

func timeLoadLocation(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}
