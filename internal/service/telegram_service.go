package service

import (
	"context"
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"go.uber.org/zap"
)

// Compile-time check: TelegramService must implement model.MessageSender.
var _ model.MessageSender = (*TelegramService)(nil)

// TelegramService handles outbound messages and webhook registration for Telegram.
type TelegramService struct {
	bot    *tgbotapi.BotAPI
	logger *zap.Logger
}

// NewTelegramService creates a TelegramService using the given bot token.
func NewTelegramService(token string, logger *zap.Logger) (*TelegramService, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init: %w", err)
	}
	logger.Info("telegram bot authenticated", zap.String("username", "@"+bot.Self.UserName))
	return &TelegramService{bot: bot, logger: logger}, nil
}

// SetWebhook registers the given HTTPS URL as the Telegram webhook endpoint.
// Call this once after the HTTP server is ready to receive.
func (t *TelegramService) SetWebhook(webhookURL string) error {
	wh, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("build webhook config: %w", err)
	}
	wh.DropPendingUpdates = true
	resp, err := t.bot.Request(wh)
	if err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}
	if !resp.Ok {
		return fmt.Errorf("set webhook failed (code %d): %s", resp.ErrorCode, resp.Description)
	}
	t.logger.Info("telegram webhook registered", zap.String("url", webhookURL))
	return nil
}

// DeleteWebhook removes any registered webhook (useful for clean teardown).
func (t *TelegramService) DeleteWebhook() {
	_, _ = t.bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
}

// ProcessUpdate parses a raw Telegram Update and returns a generic IncomingMessage.
// Returns (msg, true) to signal the update should be skipped (not a processable message).
func (t *TelegramService) ProcessUpdate(update tgbotapi.Update) (model.IncomingMessage, bool) {
	msg := update.Message
	if msg == nil {
		return model.IncomingMessage{}, true // skip non-message updates (callbacks, edits, etc.)
	}

	// Skip group chats — handle personal chat only
	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		return model.IncomingMessage{}, true
	}

	chatID := strconv.FormatInt(msg.Chat.ID, 10)

	senderName := ""
	if msg.From != nil {
		senderName = msg.From.FirstName
		if msg.From.LastName != "" {
			senderName += " " + msg.From.LastName
		}
	}

	incoming := model.IncomingMessage{
		Platform:   model.PlatformTelegram,
		Sender:     chatID,
		SenderName: senderName,
		Message:    msg.Text,
	}

	// Photo: Telegram sends an array of sizes; pick the largest.
	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		fileURL, err := t.bot.GetFileDirectURL(largest.FileID)
		if err != nil {
			t.logger.Warn("failed to get photo URL", zap.Error(err))
		} else {
			incoming.AttachmentURL = fileURL
			incoming.MimeType = "image/jpeg"
			if incoming.Message == "" {
				incoming.Message = msg.Caption
			}
		}
	}

	// Document (any file, including images sent as files)
	if msg.Document != nil {
		fileURL, err := t.bot.GetFileDirectURL(msg.Document.FileID)
		if err != nil {
			t.logger.Warn("failed to get document URL", zap.Error(err))
		} else {
			incoming.AttachmentURL = fileURL
			incoming.MimeType = msg.Document.MimeType
			if incoming.Message == "" {
				incoming.Message = msg.Caption
			}
		}
	}

	// Location: Telegram sends GPS location coordinates
	if msg.Location != nil {
		lat := msg.Location.Latitude
		lng := msg.Location.Longitude
		incoming.Latitude = &lat
		incoming.Longitude = &lng
		if incoming.Message == "" {
			incoming.Message = fmt.Sprintf("LOC:%f,%f", lat, lng)
		}
	}

	return incoming, false
}

// ─── model.MessageSender implementation ──────────────────────────────────────

// SendText sends a plain text message to the given Telegram chat ID (as string).
// The platform argument is accepted to satisfy model.MessageSender.
func (t *TelegramService) SendText(ctx context.Context, _ model.Platform, to, message string) error {
	chatID, err := strconv.ParseInt(to, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat id %q: %w", to, err)
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err = t.bot.Send(msg)
	if err != nil {
		t.logger.Warn("telegram SendText failed", zap.String("to", to), zap.Error(err))
		return fmt.Errorf("telegram send text: %w", err)
	}
	t.logger.Debug("telegram message sent", zap.String("to", to))
	return nil
}

// SendMedia sends a photo or document to the given Telegram chat ID (as string).
// Images (mimeType starting with "image/") are sent as photos; all others as documents.
// The platform argument is accepted to satisfy model.MessageSender.
func (t *TelegramService) SendMedia(ctx context.Context, _ model.Platform, to, caption, mediaURL, filename, mimeType string) error {
	chatID, err := strconv.ParseInt(to, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat id %q: %w", to, err)
	}

	isImage := len(mimeType) >= 6 && mimeType[:6] == "image/"
	if isImage {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(mediaURL))
		photo.Caption = caption
		photo.ParseMode = tgbotapi.ModeMarkdown
		_, err = t.bot.Send(photo)
	} else {
		doc := tgbotapi.NewDocument(chatID, tgbotapi.FileURL(mediaURL))
		doc.Caption = caption
		doc.ParseMode = tgbotapi.ModeMarkdown
		_, err = t.bot.Send(doc)
	}

	if err != nil {
		t.logger.Warn("telegram SendMedia failed", zap.String("to", to), zap.Error(err))
		return fmt.Errorf("telegram send media: %w", err)
	}
	return nil
}

// SendDocumentBytes sends in-memory document file bytes (e.g. generated Excel files) directly to Telegram.
func (t *TelegramService) SendDocumentBytes(ctx context.Context, _ model.Platform, to string, fileBytes []byte, filename, caption string) error {
	chatID, err := strconv.ParseInt(to, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat id %q: %w", to, err)
	}

	fileBytesData := tgbotapi.FileBytes{
		Name:  filename,
		Bytes: fileBytes,
	}

	doc := tgbotapi.NewDocument(chatID, fileBytesData)
	doc.Caption = caption
	doc.ParseMode = tgbotapi.ModeMarkdown
	_, err = t.bot.Send(doc)
	if err != nil {
		t.logger.Warn("telegram SendDocumentBytes failed", zap.String("to", to), zap.Error(err))
		return fmt.Errorf("telegram send document bytes: %w", err)
	}
	return nil
}
