package telegram

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"olexsmir.xyz/rss-tools/app"
)

type telegram struct {
	db        *app.Bucket
	messages  *app.Bucket
	client    *http.Client
	tg        *TelegramSDK
	allowedID int64
}

func Register(a *app.App) error {
	db, err := a.Bucket("telegram")
	if err != nil {
		return err
	}

	messages, err := a.Bucket("telegram:messages")
	if err != nil {
		return err
	}

	t := &telegram{
		db:        db,
		messages:  messages,
		client:    a.Client,
		tg:        NewSDK(a.Client, a.Config.TGToken),
		allowedID: a.Config.TGUserID,
	}

	a.AddWorker(t.worker)
	a.Route("GET /telegram", t.handler)
	return nil
}

func (t *telegram) handler(w http.ResponseWriter, r *http.Request) {
	// todo: cache feed contruction
	// todo: dont include messages older than N days

	messages, err := t.loadMessages()
	if err != nil {
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		return
	}

	feed := app.NewFeed("Telegram feed", "telegram-feed")
	for _, m := range messages {
		feed.Add(feedEntryFromMessage(m))
	}

	w.WriteHeader(http.StatusOK)
	feed.Render(w)
}

func (t *telegram) worker(ctx context.Context) error {
	offset, err := t.loadOffset()
	if err != nil {
		return err
	}

	for {
		updates, err := t.tg.GetUpdates(ctx, offset)
		if err != nil {
			slog.ErrorContext(ctx, "getUpdates failed", "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}
		}

		for _, u := range updates {
			if u.Message != nil && u.Message.From != nil {
				slog.InfoContext(ctx, "message from", "user_id", u.Message.From.ID, "username", u.Message.From.Username, "msg", messageText(u.Message))
			}

			if u.Message == nil || u.Message.From == nil || u.Message.From.ID != t.allowedID {
				offset = u.UpdateID + 1
				continue
			}

			if err := t.saveMessage(u.Message); err != nil {
				slog.ErrorContext(ctx, "failed to save message", "err", err)
			}

			if err := t.tg.SetReaction(ctx, u.Message.From.ID, u.Message.MessageID, "👍"); err != nil {
				slog.ErrorContext(ctx, "failed to set reaction", "err", err)
			}

			offset = u.UpdateID + 1
		}

		if err := t.saveOffset(offset); err != nil {
			slog.ErrorContext(ctx, "failed to save offset", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (t *telegram) saveOffset(offset int64) error {
	return t.db.Set([]byte("offset"), binary.BigEndian.AppendUint64(nil, uint64(offset)))
}

func (t *telegram) loadOffset() (int64, error) {
	val, err := t.db.Get([]byte("offset"))
	if err != nil || val == nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(val)), nil
}

func (t *telegram) saveMessage(m *Message) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(m); err != nil {
		return err
	}
	key := binary.BigEndian.AppendUint64(nil, uint64(m.MessageID))
	return t.messages.Set(key, buf.Bytes())
}

func (t *telegram) loadMessages() ([]*Message, error) {
	var messages []*Message
	err := t.messages.ForEach(func(k, v []byte) error {
		var m Message
		if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&m); err != nil {
			return err
		}
		messages = append(messages, &m)
		return nil
	})
	return messages, err
}

func feedEntryFromMessage(m *Message) app.FeedEntry {
	updated := time.Unix(m.Date, 0)
	text := messageText(m)
	if m.PhotoBase64 == "" {
		title := text
		if len(title) > 64 {
			title = title[:64] + "..."
		}
		return app.FeedEntry{
			Title:   title,
			ID:      fmt.Sprintf("telegram-%d", m.MessageID),
			Content: text,
			Updated: updated,
		}
	}

	parts := make([]string, 0, 2)
	if t := strings.TrimSpace(text); t != "" {
		parts = append(parts, "<p>"+html.EscapeString(t)+"</p>")
	}
	mimeType := m.PhotoMIMEType
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	parts = append(parts, fmt.Sprintf(`<p><img src="data:%s;base64,%s" alt="telegram image"/></p>`, mimeType, m.PhotoBase64))

	return app.FeedEntry{
		Title:       fmt.Sprintf("🖼️ [%s]", updated.Format("2006-01-02")),
		ID:          fmt.Sprintf("telegram-%d", m.MessageID),
		Content:     strings.Join(parts, ""),
		ContentType: "html",
		Updated:     updated,
	}
}

func messageText(m *Message) string {
	if m == nil {
		return ""
	}
	if caption := strings.TrimSpace(m.Caption); caption != "" {
		return m.Caption
	}
	return m.Text
}
