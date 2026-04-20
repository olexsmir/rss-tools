// source for https://ztoe.com.ua/unhooking-search.php
package ztoe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"

	"olexsmir.xyz/rss-tools/app"
)

type ztoe struct {
	get func(ctx context.Context, url string) (*http.Response, error)
}

const sourceURL = "https://ztoe.com.ua/unhooking-search.php"

func Register(a *app.App) error {
	z := ztoe{get: a.Get}
	a.Route("GET /ztoe/{group}/{subgroup}", z.handler(sourceURL))
	return nil
}

func (z *ztoe) handler(scheduleURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		group := r.PathValue("group")
		subgroup := r.PathValue("subgroup")

		schedule, err := z.fetchSchedule(r.Context(), scheduleURL)
		if err != nil {
			http.Error(w, "failed to fetch schedule", http.StatusBadGateway)
			return
		}

		row, ok := schedule.Rows[group+"."+subgroup]
		if !ok {
			http.Error(w, "group/subgroup not found", http.StatusNotFound)
			return
		}

		slots := make([]slot, 0, len(schedule.TimeSlots))
		for i, t := range schedule.TimeSlots {
			slots = append(slots, slot{Range: t, Outage: i < len(row) && row[i]})
		}

		feed := app.NewFeed(
			fmt.Sprintf("ZTOE power outages for %s.%s", group, subgroup),
			fmt.Sprintf("ztoe-%s-%s", group, subgroup))

		for _, interval := range buildOutageIntervals(slots) {
			feed.Add(app.FeedEntry{
				Title:   fmt.Sprintf("Power outage %s-%s", interval.Start, interval.End),
				ID:      fmt.Sprintf("ztoe-%s-%s-%s-%s-%s", group, subgroup, schedule.Date, strings.ReplaceAll(interval.Start, ":", ""), strings.ReplaceAll(interval.End, ":", "")),
				Content: fmt.Sprintf("Date: %s\nGroup: %s.%s\nTime: %s-%s", schedule.Date, group, subgroup, interval.Start, interval.End),
				Updated: intervalTime(schedule.Date, interval.Start),
			})
		}
		if err := feed.Render(w); err != nil {
			http.Error(w, "failed to render feed", http.StatusInternalServerError)
			return
		}
	}
}

var (
	timeSlotRe = regexp.MustCompile(`^\d{2}:\d{2}-\d{2}:\d{2}$`)
	dateRe     = regexp.MustCompile(`\d{2}\.\d{2}\.\d{4}`)
	subgroupRe = regexp.MustCompile(`^\d+\.\d+$`)
	bgColorRe  = regexp.MustCompile(`(?i)background(?:-color)?\s*:\s*([^;]+)`)
)

type slot struct {
	Range  string `json:"range"`
	Outage bool   `json:"outage"`
}

type outageInterval struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type parsedSchedule struct {
	Date      string
	TimeSlots []string
	Rows      map[string][]bool
}

func (z *ztoe) fetchSchedule(ctx context.Context, scheduleURL string) (*parsedSchedule, error) {
	res, err := z.get(ctx, scheduleURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", res.Status)
	}

	return parseSchedule(res.Body, res.Header.Get("Content-Type"))
}

func parseSchedule(r io.Reader, contentType string) (*parsedSchedule, error) {
	if contentType == "" {
		contentType = "text/html"
	}
	decoded, err := charset.NewReader(r, contentType)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(decoded)
	if err != nil {
		return nil, err
	}

	table := findScheduleTable(doc)
	if table == nil {
		return nil, errors.New("failed to locate schedule table")
	}

	timeSlots := extractTimeSlots(table)
	if len(timeSlots) == 0 {
		return nil, errors.New("failed to parse schedule time slots")
	}

	rows := extractRows(table, len(timeSlots))
	if len(rows) == 0 {
		return nil, errors.New("failed to parse schedule rows")
	}

	return &parsedSchedule{
		Date:      extractDate(table),
		TimeSlots: timeSlots,
		Rows:      rows,
	}, nil
}

func findScheduleTable(doc *goquery.Document) *goquery.Selection {
	var found *goquery.Selection
	doc.Find("table").EachWithBreak(func(_ int, table *goquery.Selection) bool {
		if len(extractTimeSlots(table)) >= 48 {
			found = table
			return false
		}
		return true
	})
	return found
}

func extractDate(table *goquery.Selection) string {
	date := ""
	table.Find("td,th").EachWithBreak(func(_ int, cell *goquery.Selection) bool {
		match := dateRe.FindString(normalizeWhitespace(cell.Text()))
		if match == "" {
			return true
		}
		date = match
		return false
	})
	return date
}

func extractTimeSlots(table *goquery.Selection) []string {
	slots := make([]string, 0, 48)
	seen := make(map[string]struct{}, 48)
	table.Find("td,th").Each(func(_ int, cell *goquery.Selection) {
		text := normalizeWhitespace(cell.Text())
		if !timeSlotRe.MatchString(text) {
			return
		}
		if _, ok := seen[text]; ok {
			return
		}
		seen[text] = struct{}{}
		slots = append(slots, text)
	})
	return slots
}

func extractRows(table *goquery.Selection, slotCount int) map[string][]bool {
	rows := make(map[string][]bool)
	table.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		tds := tr.ChildrenFiltered("td")
		if tds.Length() == 0 {
			return
		}

		subgroup := ""
		subgroupIdx := -1
		tds.EachWithBreak(func(i int, td *goquery.Selection) bool {
			text := normalizeWhitespace(td.Text())
			if !subgroupRe.MatchString(text) {
				return true
			}
			subgroup = text
			subgroupIdx = i
			return false
		})
		if subgroup == "" {
			return
		}

		slots := make([]bool, 0, slotCount)
		for i := subgroupIdx + 1; i < tds.Length() && len(slots) < slotCount; i++ {
			td := tds.Eq(i)
			style, ok := td.Attr("style")
			if !ok {
				continue
			}
			color, ok := extractBackgroundColor(style)
			if !ok {
				continue
			}
			slots = append(slots, isOutageColor(color))
		}

		if len(slots) == slotCount {
			rows[subgroup] = slots
		}
	})
	return rows
}

func extractBackgroundColor(style string) (string, bool) {
	match := bgColorRe.FindStringSubmatch(style)
	if len(match) < 2 {
		return "", false
	}
	color := strings.ToLower(strings.TrimSpace(match[1]))
	return strings.ReplaceAll(color, " ", ""), true
}

func isOutageColor(color string) bool {
	switch color {
	case "", "white", "#fff", "#ffffff", "rgb(255,255,255)", "rgba(255,255,255,1)":
		return false
	default:
		return !strings.Contains(color, "255,255,255")
	}
}

func buildOutageIntervals(slots []slot) []outageInterval {
	intervals := make([]outageInterval, 0)
	var current outageInterval
	active := false

	for _, slot := range slots {
		start, end, ok := strings.Cut(slot.Range, "-")
		if !ok {
			continue
		}

		if slot.Outage {
			if !active {
				current = outageInterval{Start: start, End: end}
				active = true
				continue
			}
			if current.End == start {
				current.End = end
				continue
			}
			intervals = append(intervals, current)
			current = outageInterval{Start: start, End: end}
			continue
		}

		if active {
			intervals = append(intervals, current)
			active = false
		}
	}
	if active {
		intervals = append(intervals, current)
	}
	return intervals
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func intervalTime(date, hhmm string) time.Time {
	day, err := time.ParseInLocation("02.01.2006", date, time.Local)
	if err != nil {
		return time.Now()
	}
	clock, err := time.Parse("15:04", hhmm)
	if err != nil {
		return day
	}
	return time.Date(
		day.Year(), day.Month(), day.Day(),
		clock.Hour(), clock.Minute(), 0, 0,
		day.Location(),
	)
}
