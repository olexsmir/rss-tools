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
	get       func(context.Context, string) (*http.Response, error)
	tg        *TelegramSDK
	allowedID int64
	logger    *slog.Logger
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
		get:       a.Get,
		tg:        NewSDK(a.Client, a.Config.TGToken),
		allowedID: a.Config.TGUserID,
		logger:    a.Logger,
	}

	a.AddWorker(t.worker)
	a.Route("GET /telegram", t.handler)
	a.Logger.Info("telegram source registered")
	return nil
}

func (t *telegram) handler(w http.ResponseWriter, r *http.Request) {
	// todo: cache feed contruction
	// todo: dont include messages older than N days

	messages, err := t.loadMessages(r.Context())
	if err != nil {
		http.Error(w, "failed to load messages", http.StatusInternalServerError)
		return
	}

	feed := app.NewFeed("Telegram feed", "telegram-feed")
	for _, m := range messages {
		if changed := t.enrichMessageWithLinkTitles(r.Context(), m); changed {
			if err := t.saveMessage(m); err != nil {
				http.Error(w, "failed to update cached titles", http.StatusInternalServerError)
				return
			}
		}
	}

	for _, m := range groupMessages(messages) {
		feed.Add(feedEntryFromMessage(m))
	}

	if err := feed.Render(w); err != nil {
		http.Error(w, "failed to render feed", http.StatusInternalServerError)
		return
	}
}

func (t *telegram) worker(ctx context.Context) error {
	t.logger.Info("starting telegram bot")

	offset, err := t.loadOffset()
	if err != nil {
		return err
	}

	for {
		updates, err := t.tg.GetUpdates(ctx, offset)
		if err != nil {
			t.logger.ErrorContext(ctx, "getUpdates failed", "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}
		}

		for _, u := range updates {
			if u.Message != nil && u.Message.From != nil {
				t.logger.InfoContext(ctx, "message from", "user_id", u.Message.From.ID, "username", u.Message.From.Username, "msg", messageText(u.Message))
			}

			if u.Message == nil || u.Message.From == nil || u.Message.From.ID != t.allowedID {
				offset = u.UpdateID + 1
				continue
			}

			_ = t.enrichMessageWithLinkTitles(ctx, u.Message)

			if err := t.saveMessage(u.Message); err != nil {
				t.logger.ErrorContext(ctx, "failed to save message", "err", err)
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

func (t *telegram) loadMessages(ctx context.Context) ([]*Message, error) {
	var messages []*Message
	err := t.messages.ForEach(func(k, v []byte) error {
		var m Message
		if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&m); err != nil {
			t.logger.WarnContext(ctx, "failed to decode telegram message, skipping", "key", fmt.Sprintf("%x", k), "err", err)
			return nil
		}
		messages = append(messages, &m)
		return nil
	})
	return messages, err
}

func groupMessages(messages []*Message) []*Message {
	if len(messages) == 0 {
		return messages
	}

	groups := make(map[string]*Message)
	out := make([]*Message, 0, len(messages))
	for _, m := range messages {
		if m == nil || strings.TrimSpace(m.MediaGroupID) == "" {
			out = append(out, m)
			continue
		}

		group, ok := groups[m.MediaGroupID]
		if !ok {
			group = &Message{
				MessageID:    m.MessageID,
				From:         m.From,
				Chat:         m.Chat,
				Text:         m.Text,
				Caption:      m.Caption,
				Date:         m.Date,
				MediaGroupID: m.MediaGroupID,
				LinkTitles:   m.LinkTitles,
			}
			groups[m.MediaGroupID] = group
			out = append(out, group)
		}

		if m.MessageID != 0 && (group.MessageID == 0 || m.MessageID < group.MessageID) {
			group.MessageID = m.MessageID
		}
		if m.Date != 0 && (group.Date == 0 || m.Date < group.Date) {
			group.Date = m.Date
		}
		if strings.TrimSpace(messageText(group)) == "" && strings.TrimSpace(messageText(m)) != "" {
			group.Caption = m.Caption
			group.Text = m.Text
			if len(m.LinkTitles) > 0 {
				group.LinkTitles = m.LinkTitles
			}
		} else if len(group.LinkTitles) == 0 && len(m.LinkTitles) > 0 {
			group.LinkTitles = m.LinkTitles
		}

		group.PhotoAttachments = append(group.PhotoAttachments, messagePhotos(m)...)
		if group.PhotoBase64 == "" && m.PhotoBase64 != "" {
			group.PhotoBase64 = m.PhotoBase64
			group.PhotoMIMEType = m.PhotoMIMEType
		}
	}
	return out
}

func (t *telegram) enrichMessageWithLinkTitles(ctx context.Context, m *Message) bool {
	text := messageText(m)
	if !isSingleLinkMessage(text) {
		return false
	}

	links := normalizeLinks(messageLinks(text))
	if len(links) == 0 {
		return false
	}
	if m.LinkTitles == nil {
		m.LinkTitles = make(map[string]string, len(links))
	}

	changed := false
	for _, link := range links {
		cachedTitle := normalizePageTitle(m.LinkTitles[link])
		if isMeaningfulPageTitle(cachedTitle) {
			continue
		}
		if cachedTitle != "" {
			delete(m.LinkTitles, link)
			changed = true
		}
		title, err := fetchPageTitle(ctx, t.get, link)
		if err != nil {
			t.logger.WarnContext(ctx, "failed to lookup page title", "url", link, "err", err)
			continue
		}
		m.LinkTitles[link] = title
		changed = true
	}
	return changed
}

func feedEntryFromMessage(m *Message) app.FeedEntry {
	updated := time.Unix(m.Date, 0)
	text := normalizeMessageText(messageText(m))
	normalizedLinks := normalizeLinks(messageLinks(text))
	entryID := fmt.Sprintf("telegram-%d", m.MessageID)
	if videoID, ok := firstYouTubeVideoID(normalizedLinks); ok {
		entryID = "yt:video:" + videoID
	}

	photos := messagePhotos(m)
	if len(photos) == 0 {
		title := text
		if isSingleLinkMessage(text) {
			for _, link := range normalizedLinks {
				if t := strings.TrimSpace(m.LinkTitles[link]); t != "" {
					title = t
					break
				}
			}
		}
		if len(title) > 64 {
			title = title[:64] + "..."
		}

		content := text
		contentType := ""
		if len(normalizedLinks) > 0 {
			content, _ = linkifyMessageText(text)
			content = preserveLineBreaks(content)
			contentType = "html"
		} else if strings.Contains(text, "\n") {
			content = preserveLineBreaks(html.EscapeString(text))
			contentType = "html"
		}

		return app.FeedEntry{
			Title:       title,
			ID:          entryID,
			Links:       feedLinks(normalizedLinks),
			Content:     content,
			Updated:     updated,
			ContentType: contentType,
		}
	}

	parts := make([]string, 0, 1+len(photos))
	if t := strings.TrimSpace(text); t != "" {
		linkified, _ := linkifyMessageText(text)
		linkified = preserveLineBreaks(linkified)
		parts = append(parts, "<p>"+linkified+"</p>")
	}
	for _, photo := range photos {
		if photo.Base64 == "" {
			continue
		}
		mimeType := photo.MIMEType
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
		parts = append(parts, fmt.Sprintf(`<p><img src="data:%s;base64,%s" alt="telegram image"/></p>`, mimeType, photo.Base64))
	}

	return app.FeedEntry{
		Title:       fmt.Sprintf("🖼️ [%s]", updated.Format("2006-01-02")),
		ID:          entryID,
		Links:       feedLinks(normalizedLinks),
		Content:     strings.Join(parts, ""),
		ContentType: "html",
		Updated:     updated,
	}
}

func isSingleLinkMessage(text string) bool {
	links := findLinks(text)
	if len(links) != 1 {
		return false
	}
	link := links[0]
	if strings.TrimSpace(text[:link.start]) != "" {
		return false
	}
	after := strings.TrimSpace(text[link.end:])
	return trailingPunctRe.ReplaceAllString(after, "") == ""
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

func normalizeMessageText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func preserveLineBreaks(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	return strings.ReplaceAll(text, "\n", "<br/>")
}

func messagePhotos(m *Message) []PhotoAttachment {
	if m == nil {
		return nil
	}
	if len(m.PhotoAttachments) > 0 {
		out := make([]PhotoAttachment, len(m.PhotoAttachments))
		copy(out, m.PhotoAttachments)
		return out
	}
	if m.PhotoBase64 == "" {
		return nil
	}
	mimeType := m.PhotoMIMEType
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return []PhotoAttachment{{Base64: m.PhotoBase64, MIMEType: mimeType}}
}
