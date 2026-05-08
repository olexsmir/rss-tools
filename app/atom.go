package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	atomNamespace  = "http://www.w3.org/2005/Atom"
	xhtmlNamespace = "http://www.w3.org/1999/xhtml"
	defaultAuthor  = "rss-tools"
)

type AtomFeed struct {
	XMLName  xml.Name     `xml:"feed"`
	XMLNS    string       `xml:"xmlns,attr"`
	Title    string       `xml:"title"`
	ID       string       `xml:"id"`
	Updated  string       `xml:"updated"`
	Authors  []AtomPerson `xml:"author,omitempty"`
	Subtitle string       `xml:"subtitle,omitempty"`
	Entries  []AtomEntry  `xml:"entry"`
}

type AtomEntry struct {
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []AtomLink  `xml:"link,omitempty"`
	Content AtomContent `xml:"content"`
}

type AtomContent struct {
	XMLName xml.Name `xml:"content"`
	Type    string   `xml:"type,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

func (c AtomContent) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	contentType := c.Type
	if contentType == "" {
		contentType = "text"
	}

	start.Name = xml.Name{Local: "content"}
	start.Attr = append(start.Attr, xml.Attr{
		Name:  xml.Name{Local: "type"},
		Value: contentType,
	})

	if err := e.EncodeToken(start); err != nil {
		return err
	}

	if contentType == "xhtml" {
		if err := validateXHTMLFragment(c.Value); err != nil {
			return err
		}

		if err := e.Encode(xhtmlDiv{
			XMLNS: xhtmlNamespace,
			Inner: c.Value,
		}); err != nil {
			return err
		}
	} else {
		if err := e.EncodeToken(xml.CharData([]byte(c.Value))); err != nil {
			return err
		}
	}

	if err := e.EncodeToken(start.End()); err != nil {
		return err
	}
	return e.Flush()
}

type xhtmlDiv struct {
	XMLName xml.Name `xml:"div"`
	XMLNS   string   `xml:"xmlns,attr"`
	Inner   string   `xml:",innerxml"`
}

func validateXHTMLFragment(fragment string) error {
	wrapped := fmt.Sprintf(`<div xmlns="%s">%s</div>`, xhtmlNamespace, fragment)
	dec := xml.NewDecoder(strings.NewReader(wrapped))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid xhtml content: %w", err)
		}
	}
}

type AtomPerson struct {
	Name string `xml:"name"`
}

type AtomLink struct {
	Rel    string `xml:"rel,attr,omitempty"`
	Type   string `xml:"type,attr,omitempty"`
	Length string `xml:"length,attr,omitempty"`
	Href   string `xml:"href,attr"`
}

type FeedEntry struct {
	Title       string
	ID          string
	Links       []FeedLink
	Content     string
	ContentType string // "text", "html", or "xhtml"; defaults to "text"
	Updated     time.Time
}

type FeedLink struct {
	Rel    string
	Type   string
	Length string
	Href   string
}

type FeedBuilder struct{ f AtomFeed }

func NewFeed(title, id string) *FeedBuilder {
	return &FeedBuilder{f: AtomFeed{
		XMLNS:   atomNamespace,
		Title:   title,
		ID:      id,
		Updated: time.Now().Format(time.RFC3339),
		Authors: []AtomPerson{{Name: defaultAuthor}},
	}}
}

func (f *FeedBuilder) WithSubtitle(subtitle string) *FeedBuilder {
	f.f.Subtitle = subtitle
	return f
}

func (f *FeedBuilder) WithAuthor(name string) *FeedBuilder {
	name = strings.TrimSpace(name)
	if name == "" {
		return f
	}
	f.f.Authors = []AtomPerson{{Name: name}}
	return f
}

func (f *FeedBuilder) WithUpdated(updated time.Time) *FeedBuilder {
	if !updated.IsZero() {
		f.f.Updated = updated.Format(time.RFC3339)
	}
	return f
}

func (f *FeedBuilder) Add(entry FeedEntry) *FeedBuilder {
	if entry.Updated.IsZero() {
		entry.Updated = time.Now()
	}
	if entry.ID == "" {
		hash := sha1.Sum(fmt.Appendf(nil, "%s|%s|%s", entry.Title, entry.Content, entry.Updated.Format(time.RFC3339Nano)))
		entry.ID = fmt.Sprintf("urn:sha1:%x", hash)
	}

	contentType := entry.ContentType
	if contentType == "" {
		contentType = "text"
	}

	links := make([]AtomLink, 0, len(entry.Links))
	for _, link := range entry.Links {
		if link.Href == "" {
			continue
		}
		links = append(links, AtomLink(link))
	}

	f.f.Entries = append(f.f.Entries, AtomEntry{
		Title:   entry.Title,
		ID:      entry.ID,
		Updated: entry.Updated.Format(time.RFC3339),
		Links:   links,
		Content: AtomContent{
			Type:  contentType,
			Value: entry.Content,
		},
	})

	feedUpdated, err := time.Parse(time.RFC3339, f.f.Updated)
	if err != nil || entry.Updated.After(feedUpdated) {
		f.f.Updated = entry.Updated.Format(time.RFC3339)
	}
	return f
}

func (f *FeedBuilder) SetUpdated(updated time.Time) *FeedBuilder {
	if !updated.IsZero() {
		f.f.Updated = updated.Format(time.RFC3339)
	}
	return f
}

func (f *FeedBuilder) WriteTo(w io.Writer) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(f.f)
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
