package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AtomFeed struct {
	XMLName  xml.Name    `xml:"feed"`
	XMLNS    string      `xml:"xmlns,attr"`
	Title    string      `xml:"title"`
	ID       string      `xml:"id"`
	Updated  string      `xml:"updated"`
	Subtitle string      `xml:"subtitle,omitempty"`
	Entries  []AtomEntry `xml:"entry"`
}

type AtomEntry struct {
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Content AtomContent `xml:"content"`
}

type AtomContent struct {
	XMLName xml.Name `xml:"content"`
	Type    string   `xml:"type,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

type FeedEntry struct {
	Title       string
	ID          string
	Content     string
	ContentType string // "text" or "html", defaults to "text"
	Updated     time.Time
}

type FeedOption func(*AtomFeed)

func WithFeedSubtitle(subtitle string) FeedOption {
	return func(f *AtomFeed) {
		f.Subtitle = subtitle
	}
}

func WithFeedUpdated(updated time.Time) FeedOption {
	return func(f *AtomFeed) {
		if !updated.IsZero() {
			f.Updated = updated.Format(time.RFC3339)
		}
	}
}

type FeedBuilder struct {
	feed AtomFeed
}

func NewFeed(title, id string, opts ...FeedOption) *FeedBuilder {
	builder := &FeedBuilder{feed: AtomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		Title:   title,
		ID:      id,
		Updated: time.Now().Format(time.RFC3339),
	}}
	for _, opt := range opts {
		opt(&builder.feed)
	}
	return builder
}

func (f *FeedBuilder) Add(title, id, content string, date time.Time) *FeedBuilder {
	return f.AddEntry(FeedEntry{
		Title:   title,
		ID:      id,
		Content: content,
		Updated: date,
	})
}

func (f *FeedBuilder) AddText(title, content string, updated time.Time) *FeedBuilder {
	return f.AddEntry(FeedEntry{
		Title:   title,
		Content: content,
		Updated: updated,
	})
}

func (f *FeedBuilder) AddEntry(entry FeedEntry) *FeedBuilder {
	if entry.Updated.IsZero() {
		entry.Updated = time.Now()
	}
	if entry.ID == "" {
		hash := sha1.Sum(fmt.Appendf(nil, "%s|%s|%s", entry.Title, entry.Content, entry.Updated.Format(time.RFC3339Nano)))
		entry.ID = fmt.Sprintf("urn:sha1:%x", hash)
	}

	f.feed.Entries = append(f.feed.Entries, AtomEntry{
		Title:   entry.Title,
		ID:      entry.ID,
		Updated: entry.Updated.Format(time.RFC3339),
		Content: AtomContent{
			Type:  contentType,
			Value: entry.Content,
		},
	})

	feedUpdated, err := time.Parse(time.RFC3339, f.feed.Updated)
	if err != nil || entry.Updated.After(feedUpdated) {
		f.feed.Updated = entry.Updated.Format(time.RFC3339)
	}
	return f
}

func (f *FeedBuilder) SetUpdated(updated time.Time) *FeedBuilder {
	if !updated.IsZero() {
		f.feed.Updated = updated.Format(time.RFC3339)
	}
	return f
}

func (f *FeedBuilder) WriteTo(w io.Writer) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(f.feed)
}

func (f *FeedBuilder) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *FeedBuilder) Render(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	return f.WriteTo(w)
}
