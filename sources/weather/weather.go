package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"olexsmir.xyz/rss-tools/app"
)

const (
	forecastAPIURL   = "https://api.open-meteo.com/v1/forecast"
	airQualityAPIURL = "https://air-quality-api.open-meteo.com/v1/air-quality"
	geocodingAPIURL  = "https://nominatim.openstreetmap.org/reverse"

	clockLayout = "15:04"
	localLayout = "2006-01-02T15:04"
)

var timelineHours = []int{8, 12, 16, 20}

type weather struct {
	client        *http.Client
	forecastURL   string
	airQualityURL string
	geocodingURL  string
}

func Register(a *app.App) error {
	w := &weather{
		client:        a.Client,
		forecastURL:   forecastAPIURL,
		airQualityURL: airQualityAPIURL,
		geocodingURL:  geocodingAPIURL,
	}
	a.Route("GET /weather", w.handler)
	a.Logger.Info("weather source registered")
	return nil
}

func (w *weather) handler(rw http.ResponseWriter, r *http.Request) {
	lat, lon, err := parseCoordinates(r)
	if err != nil {
		http.Error(rw, "invalid latitude/longitude", http.StatusBadRequest)
		return
	}

	forecast, err := w.fetchForecast(r.Context(), lat, lon)
	if err != nil {
		http.Error(rw, "failed to fetch weather forecast", http.StatusBadGateway)
		return
	}

	air, err := w.fetchAirQuality(r.Context(), lat, lon)
	if err != nil {
		http.Error(rw, "failed to fetch air quality", http.StatusBadGateway)
		return
	}

	content, updated, err := buildMorningBriefing(forecast, air)
	if err != nil {
		http.Error(rw, "failed to build weather briefing", http.StatusBadGateway)
		return
	}

	place := fmt.Sprintf("%.4f,%.4f", lat, lon)
	if town, err := w.fetchTownName(r.Context(), lat, lon); err == nil && town != "" {
		place = town
	}

	feedID := weatherFeedID(lat, lon)
	feed := app.NewFeed(fmt.Sprintf("Weather forecast for %s", place), feedID).
		WithUpdated(updated)

	feed.Add(app.FeedEntry{
		Title:       "Morning weather briefing",
		ID:          fmt.Sprintf("%s-%s", feedID, updated.Format("20060102")),
		Content:     formatBriefingHTML(content),
		ContentType: "html",
		Updated:     updated,
	})

	if err := feed.Render(rw); err != nil {
		http.Error(rw, "failed to render feed", http.StatusInternalServerError)
	}
}

func parseCoordinates(r *http.Request) (float64, float64, error) {
	latRaw := strings.TrimSpace(r.URL.Query().Get("latitude"))
	lonRaw := strings.TrimSpace(r.URL.Query().Get("longitude"))
	if latRaw == "" || lonRaw == "" {
		return 0, 0, errors.New("latitude and longitude are required")
	}

	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}
	lon, err := strconv.ParseFloat(lonRaw, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}

	if lat < -90 || lat > 90 {
		return 0, 0, errors.New("latitude is out of range")
	}
	if lon < -180 || lon > 180 {
		return 0, 0, errors.New("longitude is out of range")
	}
	return lat, lon, nil
}

type forecastResponse struct {
	Timezone string `json:"timezone"`
	Current  struct {
		Time                string   `json:"time"`
		Temperature2M       *float64 `json:"temperature_2m"`
		ApparentTemperature *float64 `json:"apparent_temperature"`
	} `json:"current"`
	Daily struct {
		Time                     []string  `json:"time"`
		TemperatureMin           []float64 `json:"temperature_2m_min"`
		TemperatureMax           []float64 `json:"temperature_2m_max"`
		WeatherCode              []int     `json:"weather_code"`
		PrecipitationProbability []float64 `json:"precipitation_probability_max"`
		PrecipitationSum         []float64 `json:"precipitation_sum"`
		RainSum                  []float64 `json:"rain_sum"`
		ShowersSum               []float64 `json:"showers_sum"`
		WindSpeedMax             []float64 `json:"wind_speed_10m_max"`
		WindDirectionDominant    []float64 `json:"wind_direction_10m_dominant"`
	} `json:"daily"`
	Hourly struct {
		Time                     []string  `json:"time"`
		Temperature              []float64 `json:"temperature_2m"`
		WeatherCode              []int     `json:"weather_code"`
		PrecipitationProbability []float64 `json:"precipitation_probability"`
		WindSpeed                []float64 `json:"wind_speed_10m"`
		Humidity                 []float64 `json:"relative_humidity_2m"`
	} `json:"hourly"`
}

type airQualityResponse struct {
	Current struct {
		USAQI           *float64 `json:"us_aqi"`
		PM2_5           *float64 `json:"pm2_5"`
		PM10            *float64 `json:"pm10"`
		Ozone           *float64 `json:"ozone"`
		NitrogenDioxide *float64 `json:"nitrogen_dioxide"`
	} `json:"current"`
}

type geocodingResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Address     struct {
		City         string `json:"city"`
		Town         string `json:"town"`
		Village      string `json:"village"`
		Municipality string `json:"municipality"`
		Hamlet       string `json:"hamlet"`
		County       string `json:"county"`
	} `json:"address"`
}

func (w *weather) fetchForecast(ctx context.Context, lat, lon float64) (forecastResponse, error) {
	var payload forecastResponse

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(lat, 'f', 6, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', 6, 64))
	q.Set("timezone", "auto")
	q.Set("forecast_days", "1")
	q.Set("current", strings.Join([]string{
		"temperature_2m",
		"apparent_temperature",
	}, ","))
	q.Set("daily", strings.Join([]string{
		"temperature_2m_min",
		"temperature_2m_max",
		"weather_code",
		"precipitation_probability_max",
		"precipitation_sum",
		"rain_sum",
		"showers_sum",
		"wind_speed_10m_max",
		"wind_direction_10m_dominant",
	}, ","))
	q.Set("hourly", strings.Join([]string{
		"temperature_2m",
		"weather_code",
		"precipitation_probability",
		"wind_speed_10m",
		"relative_humidity_2m",
	}, ","))

	endpoint := w.forecastURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return payload, err
	}

	res, err := w.client.Do(req)
	if err != nil {
		return payload, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return payload, fmt.Errorf("forecast API returned %s", res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return payload, err
	}
	if len(payload.Daily.Time) == 0 || len(payload.Daily.TemperatureMin) == 0 || len(payload.Daily.TemperatureMax) == 0 {
		return payload, errors.New("forecast payload missing daily data")
	}
	if len(payload.Hourly.Time) == 0 || len(payload.Hourly.Temperature) == 0 {
		return payload, errors.New("forecast payload missing hourly data")
	}
	return payload, nil
}

func (w *weather) fetchAirQuality(ctx context.Context, lat, lon float64) (airQualityResponse, error) {
	var payload airQualityResponse

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(lat, 'f', 6, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', 6, 64))
	q.Set("timezone", "auto")
	q.Set("current", strings.Join([]string{
		"us_aqi",
		"pm2_5",
		"pm10",
		"ozone",
		"nitrogen_dioxide",
	}, ","))

	endpoint := w.airQualityURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return payload, err
	}

	res, err := w.client.Do(req)
	if err != nil {
		return payload, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return payload, fmt.Errorf("air quality API returned %s", res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func (w *weather) fetchTownName(ctx context.Context, lat, lon float64) (string, error) {
	var payload geocodingResponse

	geocodingURL := w.geocodingURL
	if geocodingURL == "" {
		geocodingURL = geocodingAPIURL
	}

	q := url.Values{}
	q.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
	q.Set("lon", strconv.FormatFloat(lon, 'f', 6, 64))
	q.Set("format", "jsonv2")
	q.Set("accept-language", "en")
	q.Set("zoom", "12")
	q.Set("addressdetails", "1")

	endpoint := geocodingURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "rss-tools/1.0 (+https://github.com/olexsmir/rss-tools)")

	res, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("geocoding API returned %s", res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}

	for _, candidate := range []string{
		payload.Address.City,
		payload.Address.Town,
		payload.Address.Village,
		payload.Address.Municipality,
		payload.Address.Hamlet,
		payload.Address.County,
		payload.Name,
		firstDisplayNamePart(payload.DisplayName),
	} {
		if town := strings.TrimSpace(candidate); town != "" {
			return town, nil
		}
	}
	return "", errors.New("town name not found")
}

func firstDisplayNamePart(displayName string) string {
	part, _, _ := strings.Cut(displayName, ",")
	return strings.TrimSpace(part)
}

func formatBriefingHTML(content string) string {
	return "<pre>" + html.EscapeString(content) + "</pre>"
}

func buildMorningBriefing(forecast forecastResponse, air airQualityResponse) (string, time.Time, error) {
	loc := time.Local
	if tz := strings.TrimSpace(forecast.Timezone); tz != "" {
		zone, err := time.LoadLocation(tz)
		if err == nil {
			loc = zone
		}
	}

	day := firstEl(forecast.Daily.Time)
	if day == "" {
		return "", time.Time{}, errors.New("missing day")
	}

	updated := time.Now().In(loc)
	if parsed, err := time.ParseInLocation(localLayout, forecast.Current.Time, loc); err == nil {
		updated = parsed
	}

	minTemp := firstEl(forecast.Daily.TemperatureMin)
	maxTemp := firstEl(forecast.Daily.TemperatureMax)
	code := firstEl(forecast.Daily.WeatherCode)
	_, dayText := weatherCodeLabel(code)

	peakPrecipHour := peakPrecipitationHour(forecast, day, loc)
	if maxProbability := firstEl(forecast.Daily.PrecipitationProbability); maxProbability >= 50 {
		switch {
		case peakPrecipHour >= 12 && peakPrecipHour <= 18:
			dayText += " with showers in the afternoon"
		case peakPrecipHour >= 5 && peakPrecipHour < 12:
			dayText += " with rain in the morning"
		default:
			dayText += " with possible rain"
		}
	}

	rainLine := fmt.Sprintf(
		"☂ %d%% chance of rain (%s)",
		int(math.Round(firstEl(forecast.Daily.PrecipitationProbability))),
		rainAmount(forecast.Daily.PrecipitationSum, forecast.Daily.RainSum, forecast.Daily.ShowersSum),
	)

	windMin, windMax := minMaxForDay(forecast.Hourly.Time, forecast.Hourly.WindSpeed, day)
	if windMin == 0 && windMax == 0 {
		windMax = firstEl(forecast.Daily.WindSpeedMax)
		windMin = windMax
	}
	windDirection := windDirectionFromDegrees(firstEl(forecast.Daily.WindDirectionDominant))
	windLine := formatWindLine(windMin, windMax, windDirection)

	lowTemp, lowTime, highTemp, highTime, ok := dayExtremes(forecast.Hourly.Time, forecast.Hourly.Temperature, day, loc)
	extremesLine := ""
	if ok {
		extremesLine = fmt.Sprintf(
			"📈 High: %s at %s  •  📉 Low: %s at %s",
			formatSignedTemperature(highTemp),
			highTime.Format(clockLayout),
			formatSignedTemperature(lowTemp),
			lowTime.Format(clockLayout),
		)
	}

	nowLine := ""
	if forecast.Current.Temperature2M != nil && forecast.Current.ApparentTemperature != nil {
		nowLine = fmt.Sprintf(
			"🌡 Now: %s (feels like %s)",
			formatSignedTemperature(*forecast.Current.Temperature2M),
			formatSignedTemperature(*forecast.Current.ApparentTemperature),
		)
	}

	humidityMin, humidityMax := minMaxForDay(forecast.Hourly.Time, forecast.Hourly.Humidity, day)
	humidityLine := ""
	if humidityMin != 0 || humidityMax != 0 {
		humidityLine = fmt.Sprintf("💧 Humidity: %d-%d%%", int(math.Round(humidityMin)), int(math.Round(humidityMax)))
	}

	airLine := formatAirQualityLine(air)
	timelineLines := timelineSummary(forecast, day, loc)
	if len(timelineLines) == 0 {
		return "", time.Time{}, errors.New("missing timeline data")
	}

	lines := []string{
		fmt.Sprintf("%s / %s  |  %s", formatSignedTemperature(minTemp), formatSignedTemperature(maxTemp), dayText),
		rainLine,
		windLine,
	}
	if extremesLine != "" {
		lines = append(lines, extremesLine)
	}
	if nowLine != "" {
		lines = append(lines, nowLine)
	}
	if humidityLine != "" {
		lines = append(lines, humidityLine)
	}
	if airLine != "" {
		lines = append(lines, airLine)
	}
	lines = append(lines, "")
	lines = append(lines, timelineLines...)
	lines = append(lines, "", "Data: Open-Meteo")

	return strings.Join(lines, "\n"), updated, nil
}

func peakPrecipitationHour(forecast forecastResponse, day string, loc *time.Location) int {
	peakHour := -1
	maxProb := -1.0
	for i, raw := range forecast.Hourly.Time {
		if i >= len(forecast.Hourly.PrecipitationProbability) || !strings.HasPrefix(raw, day+"T") {
			continue
		}
		t, err := time.ParseInLocation(localLayout, raw, loc)
		if err != nil {
			continue
		}
		prob := forecast.Hourly.PrecipitationProbability[i]
		if prob > maxProb {
			maxProb = prob
			peakHour = t.Hour()
		}
	}
	return peakHour
}

func dayExtremes(times []string, values []float64, day string, loc *time.Location) (float64, time.Time, float64, time.Time, bool) {
	if len(times) == 0 || len(values) == 0 {
		return 0, time.Time{}, 0, time.Time{}, false
	}
	minVal, maxVal := 0.0, 0.0
	var minTime, maxTime time.Time
	found := false
	for i, raw := range times {
		if i >= len(values) || !strings.HasPrefix(raw, day+"T") {
			continue
		}
		t, err := time.ParseInLocation(localLayout, raw, loc)
		if err != nil {
			continue
		}
		v := values[i]
		if !found {
			minVal, maxVal = v, v
			minTime, maxTime = t, t
			found = true
			continue
		}
		if v < minVal {
			minVal = v
			minTime = t
		}
		if v > maxVal {
			maxVal = v
			maxTime = t
		}
	}
	return minVal, minTime, maxVal, maxTime, found
}

func minMaxForDay(times []string, values []float64, day string) (float64, float64) {
	if len(times) == 0 || len(values) == 0 {
		return 0, 0
	}
	minV, maxV := 0.0, 0.0
	found := false
	for i, raw := range times {
		if i >= len(values) || !strings.HasPrefix(raw, day+"T") {
			continue
		}
		v := values[i]
		if !found {
			minV, maxV = v, v
			found = true
			continue
		}
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if !found {
		return 0, 0
	}
	return minV, maxV
}

func timelineSummary(forecast forecastResponse, day string, loc *time.Location) []string {
	out := make([]string, 0, len(timelineHours))

	type point struct {
		time  time.Time
		temp  float64
		code  int
		valid bool
	}

	points := make([]point, 0, len(forecast.Hourly.Time))
	for i, raw := range forecast.Hourly.Time {
		if i >= len(forecast.Hourly.Temperature) || !strings.HasPrefix(raw, day+"T") {
			continue
		}
		t, err := time.ParseInLocation(localLayout, raw, loc)
		if err != nil {
			continue
		}
		code := 0
		if i < len(forecast.Hourly.WeatherCode) {
			code = forecast.Hourly.WeatherCode[i]
		}
		points = append(points, point{
			time:  t,
			temp:  forecast.Hourly.Temperature[i],
			code:  code,
			valid: true,
		})
	}
	if len(points) == 0 {
		return nil
	}

	for _, targetHour := range timelineHours {
		bestIdx := -1
		bestDelta := 24
		for i, p := range points {
			delta := p.time.Hour() - targetHour
			if delta < 0 {
				delta = -delta
			}
			if delta < bestDelta {
				bestDelta = delta
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			continue
		}
		icon, text := weatherCodeLabel(points[bestIdx].code)
		out = append(out, fmt.Sprintf("%02d:00  %s  %s %s", targetHour, formatSignedTemperature(points[bestIdx].temp), icon, text))
	}
	return out
}

func weatherCodeLabel(code int) (string, string) {
	switch code {
	case 0:
		return "☀", "Clear sky"
	case 1:
		return "⛅", "Mostly clear"
	case 2:
		return "🌥", "Partly cloudy"
	case 3:
		return "☁", "Cloudy"
	case 45, 48:
		return "🌫", "Fog"
	case 51, 53, 55, 56, 57:
		return "🌦", "Drizzle"
	case 61, 63, 65, 66, 67:
		return "🌧", "Rain"
	case 71, 73, 75, 77:
		return "🌨", "Snow"
	case 80, 81, 82:
		return "🌧", "Rain showers"
	case 85, 86:
		return "🌨", "Snow showers"
	case 95, 96, 99:
		return "⛈", "Thunderstorm"
	default:
		return "🌤", "Variable clouds"
	}
}

func windDirectionFromDegrees(degrees float64) string {
	directions := []string{
		"N", "NNE", "NE", "ENE",
		"E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW",
		"W", "WNW", "NW", "NNW",
	}
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}
	idx := int(math.Round(normalized/22.5)) % len(directions)
	return directions[idx]
}

func rainAmount(precipitationSum, rainSum, showersSum []float64) string {
	precip := firstEl(precipitationSum)
	rain := firstEl(rainSum)
	showers := firstEl(showersSum)

	low := rain
	if low <= 0 {
		low = precip
	}
	high := precip
	if showers > 0 && rain > 0 {
		high = math.Max(high, rain+showers)
	}
	if high < low {
		high = low
	}
	if high <= 0 {
		return "0 mm"
	}

	lowMM := int(math.Round(low))
	highMM := int(math.Round(high))
	if lowMM == highMM {
		return fmt.Sprintf("%d mm", highMM)
	}
	return fmt.Sprintf("%d-%d mm", lowMM, highMM)
}

func formatAirQualityLine(air airQualityResponse) string {
	if air.Current.USAQI == nil && air.Current.PM2_5 == nil && air.Current.PM10 == nil {
		return ""
	}

	parts := make([]string, 0, 4)
	if air.Current.USAQI != nil {
		aqi := int(math.Round(*air.Current.USAQI))
		parts = append(parts, fmt.Sprintf("AQI %d (%s)", aqi, usAQILevel(aqi)))
	}
	if air.Current.PM2_5 != nil {
		parts = append(parts, fmt.Sprintf("PM2.5 %.1f", *air.Current.PM2_5))
	}
	if air.Current.PM10 != nil {
		parts = append(parts, fmt.Sprintf("PM10 %.1f", *air.Current.PM10))
	}
	if air.Current.Ozone != nil {
		parts = append(parts, fmt.Sprintf("O3 %.1f", *air.Current.Ozone))
	}
	if air.Current.NitrogenDioxide != nil {
		parts = append(parts, fmt.Sprintf("NO2 %.1f", *air.Current.NitrogenDioxide))
	}
	return "🌫 Air: " + strings.Join(parts, ", ")
}

func usAQILevel(v int) string {
	switch {
	case v <= 50:
		return "Good"
	case v <= 100:
		return "Moderate"
	case v <= 150:
		return "Unhealthy for sensitive groups"
	case v <= 200:
		return "Unhealthy"
	case v <= 300:
		return "Very unhealthy"
	default:
		return "Hazardous"
	}
}

func weatherFeedID(lat, lon float64) string {
	return fmt.Sprintf("weather-lat-%s-lon-%s", normalizeCoord(lat), normalizeCoord(lon))
}

func normalizeCoord(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.ReplaceAll(s, "-", "m")
	return strings.ReplaceAll(s, ".", "_")
}

func formatSignedTemperature(v float64) string {
	n := int(math.Round(v))
	if n > 0 {
		return fmt.Sprintf("+%d°", n)
	}
	return fmt.Sprintf("%d°", n)
}

func formatWindLine(minWind, maxWind float64, direction string) string {
	minV := int(math.Round(minWind))
	maxV := int(math.Round(maxWind))
	if maxV < minV {
		minV, maxV = maxV, minV
	}
	if minV == maxV {
		return fmt.Sprintf("🌬 Wind: %d km/h from %s", minV, direction)
	}
	return fmt.Sprintf("🌬 Wind: %d-%d km/h from %s", minV, maxV, direction)
}

func firstEl[T comparable](values []T) T {
	if len(values) == 0 {
		var zero T
		return zero
	}
	return values[0]
}
