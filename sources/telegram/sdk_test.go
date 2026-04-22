package telegram

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"olexsmir.xyz/x/is"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGetUpdatesHydratesPhotoBase64(t *testing.T) {
	const token = "TEST_TOKEN"
	seenGetFileForID := ""
	pngData := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x01}

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.Path, "/getUpdates"):
			return jsonResponse(`{
				"ok": true,
				"result": [{
					"update_id": 1,
					"message": {
						"message_id": 7,
						"date": 1713790000,
						"text": "photo msg",
						"photo": [
							{"file_id": "small", "width": 90, "height": 90, "file_size": 100},
							{"file_id": "large", "width": 1280, "height": 720, "file_size": 2048}
						]
					}
				}]
			}`)
		case strings.Contains(r.URL.Path, "/getFile"):
			seenGetFileForID = r.URL.Query().Get("file_id")
			return jsonResponse(`{"ok": true, "result": {"file_path": "photos/large.png"}}`)
		case strings.Contains(r.URL.Path, "/file/bot"+token+"/photos/large.png"):
			return byteResponse(pngData), nil
		default:
			t.Fatalf("unexpected request URL: %s", r.URL.String())
			return nil, nil
		}
	})}

	sdk := NewSDK(client, token)
	updates, err := sdk.GetUpdates(context.Background(), 0)
	is.Err(t, err, nil)
	is.Equal(t, 1, len(updates))

	msg := updates[0].Message
	is.Equal(t, "large", seenGetFileForID)
	is.Equal(t, base64.StdEncoding.EncodeToString(pngData), msg.PhotoBase64)
	is.Equal(t, "image/png", msg.PhotoMIMEType)
}

func jsonResponse(body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func byteResponse(data []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(data))),
		Header:     make(http.Header),
	}
}
