package app

import (
	"encoding/xml"
	"net/http"
	"time"
)

type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Entries []AtomEntry `xml:"entry"`
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

type FeedBuilder struct {
	feed AtomFeed
}

func NewFeed(title, id string) *FeedBuilder {
	return &FeedBuilder{feed: AtomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		Title:   title,
		ID:      id,
		Updated: time.Now().Format(time.RFC3339),
	}}
}

func (f *FeedBuilder) Add(title, id, content string, date time.Time) *FeedBuilder {
	f.feed.Entries = append(f.feed.Entries, AtomEntry{
		Title:   title,
		ID:      id,
		Updated: date.Format(time.RFC3339),
		Content: content,
	})
	return f
}

func (f *FeedBuilder) Render(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/atom+xml")
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(f.feed)
}
