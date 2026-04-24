package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	apiBase       = "https://api.telegram.org"
	maxPhotoBytes = 20 << 20
)

type TelegramSDK struct {
	client *http.Client
	token  string
}

func NewSDK(client *http.Client, token string) *TelegramSDK {
	return &TelegramSDK{
		token:  token,
		client: client,
	}
}

type Response[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Message struct {
	MessageID     int64             `json:"message_id"`
	From          *User             `json:"from"`
	Chat          *Chat             `json:"chat"`
	Text          string            `json:"text"`
	Caption       string            `json:"caption,omitempty"`
	Date          int64             `json:"date"`
	Photo         []PhotoSize       `json:"photo,omitempty"`
	PhotoBase64   string            `json:"photo_base64,omitempty"`
	PhotoMIMEType string            `json:"photo_mime_type,omitempty"`
	LinkTitles    map[string]string `json:"-"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

func (t *TelegramSDK) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	params := url.Values{}
	params.Set("offset", strconv.FormatInt(offset, 10))
	params.Set("timeout", "30")

	var resp Response[[]Update]
	if err := t.req(ctx, "getUpdates", params, nil, &resp); err != nil {
		return nil, err
	}

	for i := range resp.Result {
		msg := resp.Result[i].Message
		if msg == nil || len(msg.Photo) == 0 {
			continue
		}
		data, mimeType, err := t.downloadLargestPhoto(ctx, msg.Photo)
		if err != nil {
			return nil, err
		}
		msg.PhotoBase64 = base64.StdEncoding.EncodeToString(data)
		msg.PhotoMIMEType = mimeType
	}
	return resp.Result, nil
}

type messageReactionReq struct {
	Type  string `json:"type"`
	Emoji string `json:"emoji"`
}

type setReactionReq struct {
	ChatID    int64                `json:"chat_id"`
	MessageID int64                `json:"message_id"`
	Reaction  []messageReactionReq `json:"reaction"`
}

func (t *TelegramSDK) SetReaction(ctx context.Context, chatID, messageID int64, emoji string) error {
	var resp Response[bool]
	return t.req(ctx, "setMessageReaction", nil, setReactionReq{
		ChatID:    chatID,
		MessageID: messageID,
		Reaction:  []messageReactionReq{{Type: "emoji", Emoji: emoji}},
	}, &resp)
}

type tgFile struct {
	FilePath string `json:"file_path"`
}

func (t *TelegramSDK) downloadLargestPhoto(ctx context.Context, photos []PhotoSize) ([]byte, string, error) {
	if len(photos) == 0 {
		return nil, "", nil
	}

	filePath, err := t.getFilePath(ctx, photos[len(photos)-1].FileID)
	if err != nil {
		return nil, "", err
	}

	fileURL := fmt.Sprintf("%s/file/bot%s/%s", apiBase, t.token, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, "", err
	}

	res, err := t.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("photo download failed with status %d", res.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(res.Body, maxPhotoBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxPhotoBytes {
		return nil, "", fmt.Errorf("photo too large: %d bytes", len(data))
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}
	return data, mimeType, nil
}

func (t *TelegramSDK) getFilePath(ctx context.Context, fileID string) (string, error) {
	params := url.Values{}
	params.Set("file_id", fileID)

	var resp Response[tgFile]
	if err := t.req(ctx, "getFile", params, nil, &resp); err != nil {
		return "", err
	}
	return resp.Result.FilePath, nil
}

func (t *TelegramSDK) req(ctx context.Context, method string, params url.Values, body any, out any) error {
	u := fmt.Sprintf("%s/bot%s/%s", apiBase, t.token, method)
	if params != nil {
		u += "?" + params.Encode()
	}

	var req *http.Request
	var err error
	if body != nil {
		var data []byte
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
	}

	res, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return json.NewDecoder(res.Body).Decode(out)
}
