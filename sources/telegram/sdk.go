package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const apiBase = "https://api.telegram.org"

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
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
	Date      int64  `json:"date"`
}

func (t *TelegramSDK) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	params := url.Values{}
	params.Set("offset", strconv.FormatInt(offset, 10))
	params.Set("timeout", "30")

	var resp Response[[]Update]
	if err := t.req(ctx, "getUpdates", params, nil, &resp); err != nil {
		return nil, err
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
