package atom

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"
)

func NewFeed(title, id string) *Feed {
	return &Feed{
		Title:   title,
		ID:      id,
		Updated: Time(time.Now()),
		Author:  []*Person{{Name: "rss-tools"}},
	}
}

func (f *Feed) WithAuthor(name string) *Feed {
	name = strings.TrimSpace(name)
	if name == "" {
		return f
	}
	f.Author = []*Person{{Name: name}}
	return f
}

func (f *Feed) WithUpdated(updated time.Time) *Feed {
	if !updated.IsZero() {
		f.Updated = Time(updated)
	}
	return f
}

func (f *Feed) WithLink(rel, href string) *Feed {
	if href != "" {
		f.Link = append(f.Link, Link{Rel: rel, Href: href})
	}
	return f
}

func (f *Feed) Add(entry *Entry) *Feed {
	if entry != nil {
		f.Entry = append(f.Entry, entry)
	}
	return f
}

func (f *Feed) WriteTo(w io.Writer) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(f)
}

func (f *Feed) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *Feed) Render(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	return f.WriteTo(w)
}

func NewText(body, typ string) *Text {
	if body == "" && strings.TrimSpace(typ) == "" {
		return nil
	}
	if strings.TrimSpace(typ) == "" {
		typ = "text"
	}
	return &Text{Type: typ, Body: body}
}
