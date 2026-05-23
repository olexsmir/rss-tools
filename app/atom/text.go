package atom

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

func (t Text) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	contentType := strings.TrimSpace(t.Type)
	if contentType == "" {
		contentType = "text"
	}

	start.Attr = append(start.Attr, xml.Attr{
		Name:  xml.Name{Local: "type"},
		Value: contentType,
	})

	if err := e.EncodeToken(start); err != nil {
		return err
	}

	if contentType == "xhtml" {
		if err := validateXHTMLFragment(t.Body); err != nil {
			return err
		}
		if err := e.Encode(xhtmlDiv{
			XMLNS: xhtmlNamespace,
			Inner: t.Body,
		}); err != nil {
			return err
		}
	} else {
		if err := e.EncodeToken(xml.CharData([]byte(t.Body))); err != nil {
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
