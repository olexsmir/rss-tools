// Based on golang.org/x/tools/blog/atom.

package atom

import (
	"encoding/xml"
	"time"
)

const xhtmlNamespace = "http://www.w3.org/1999/xhtml"

// Feed represents an Atom feed.
type Feed struct {
	XMLName  xml.Name  `xml:"http://www.w3.org/2005/Atom feed"`
	Title    string    `xml:"title"`
	ID       string    `xml:"id"`
	Link     []Link    `xml:"link,omitempty"`
	Updated  TimeStr   `xml:"updated"`
	Author   []*Person `xml:"author,omitempty"`
	Subtitle string    `xml:"subtitle,omitempty"`
	Entry    []*Entry  `xml:"entry"`
}

// Entry represents an Atom entry.
type Entry struct {
	Title     string    `xml:"title"`
	ID        string    `xml:"id"`
	Link      []Link    `xml:"link,omitempty"`
	Published TimeStr   `xml:"published,omitempty"`
	Updated   TimeStr   `xml:"updated"`
	Author    []*Person `xml:"author,omitempty"`
	Summary   *Text     `xml:"summary,omitempty"`
	Content   *Text     `xml:"content,omitempty"`
}

// Link represents an Atom link.
type Link struct {
	Rel      string `xml:"rel,attr,omitempty"`
	Href     string `xml:"href,attr"`
	Type     string `xml:"type,attr,omitempty"`
	HrefLang string `xml:"hreflang,attr,omitempty"`
	Title    string `xml:"title,attr,omitempty"`
	Length   uint   `xml:"length,attr,omitempty"`
}

// Person represents an Atom person.
type Person struct {
	Name     string `xml:"name"`
	URI      string `xml:"uri,omitempty"`
	Email    string `xml:"email,omitempty"`
	InnerXML string `xml:",innerxml"`
}

// Text represents Atom text content.
type Text struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type TimeStr string

func Time(t time.Time) TimeStr {
	return TimeStr(t.Format(time.RFC3339))
}
