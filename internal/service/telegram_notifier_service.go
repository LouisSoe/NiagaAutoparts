package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"go.uber.org/zap"
)

// TelegramNotifierService handles outbound notifications for Order Channel and Error Channel
// using a dedicated Notification Bot (Bot 2).
type TelegramNotifierService struct {
	bot            *tgbotapi.BotAPI
	orderChannelID string
	errorChannelID string
	logger         *zap.Logger
}

// NewTelegramNotifierService creates a TelegramNotifierService using the given notifier bot token.
func NewTelegramNotifierService(token, orderChannelID, errorChannelID string, logger *zap.Logger) (*TelegramNotifierService, error) {
	if token == "" {
		logger.Warn("telegram notifier bot token is empty, channel notifications disabled")
		return &TelegramNotifierService{
			orderChannelID: orderChannelID,
			errorChannelID: errorChannelID,
			logger:         logger,
		}, nil
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram notifier bot init: %w", err)
	}
	logger.Info("telegram notifier bot authenticated", zap.String("username", "@"+bot.Self.UserName))

	return &TelegramNotifierService{
		bot:            bot,
		orderChannelID: orderChannelID,
		errorChannelID: errorChannelID,
		logger:         logger,
	}, nil
}

// SendOrderNotification broadcasts a new order alert to the Telegram Order Channel.
func (s *TelegramNotifierService) SendOrderNotification(ctx context.Context, order *model.Order) {
	if s == nil || s.bot == nil || s.orderChannelID == "" || order == nil {
		return
	}

	var sb strings.Builder
	sb.WriteString("🛒 *PESANAN BARU MASUK!*\n\n")
	sb.WriteString(fmt.Sprintf("• *No. Order:* `%s`\n", order.OrderNumber))
	sb.WriteString(fmt.Sprintf("• *Total Transaksi:* *Rp %s*\n", formatIDR(order.TotalPrice)))
	sb.WriteString(fmt.Sprintf("• *Status:* `%s`\n", order.Status))
	sb.WriteString(fmt.Sprintf("• *Metode:* `%s`\n", order.PaymentMethod.String))
	sb.WriteString(fmt.Sprintf("• *Sumber:* `%s`\n", order.Source))
	if order.Notes != "" {
		sb.WriteString(fmt.Sprintf("• *Catatan:* %s\n", order.Notes))
	}
	sb.WriteString(fmt.Sprintf("• *Waktu:* %s WIB\n\n", order.CreatedAt.Format("02 Jan 2006 15:04")))

	if len(order.Items) > 0 {
		sb.WriteString("📦 *Rincian Item Produk:*\n")
		for _, item := range order.Items {
			prodName := item.ProductName
			if prodName == "" {
				prodName = fmt.Sprintf("Produk #%d", item.ProductID)
			}
			sb.WriteString(fmt.Sprintf("  • *%s* (x%d) — Rp %s\n", prodName, item.Quantity, formatIDR(item.Subtotal)))
		}
	}

	s.sendToChannel(s.orderChannelID, sb.String())
}

// SendErrorAlert broadcasts a system error/panic alert to the Telegram Error Channel.
func (s *TelegramNotifierService) SendErrorAlert(ctx context.Context, errMessage string, contextInfo string) {
	if s == nil || s.bot == nil || s.errorChannelID == "" {
		return
	}

	// Clean/sanitize code fence backticks to prevent breaking markdown
	safeErr := strings.ReplaceAll(errMessage, "```", "'''")

	var sb strings.Builder
	sb.WriteString("🚨 *SYSTEM ERROR ALERT!*\n\n")
	sb.WriteString(fmt.Sprintf("⏰ *Waktu:* %s WIB\n", time.Now().Format("02 Jan 2006 15:04:05")))
	if contextInfo != "" {
		sb.WriteString(fmt.Sprintf("📍 *Konteks/Modul:* `%s`\n", contextInfo))
	}
	sb.WriteString(fmt.Sprintf("⚠️ *Error Details:*\n```\n%s\n```", safeErr))

	s.sendToChannel(s.errorChannelID, sb.String())
}

func (s *TelegramNotifierService) sendToChannel(channelID string, message string) {
	msg := tgbotapi.NewMessageToChannel(channelID, message)
	msg.ParseMode = tgbotapi.ModeMarkdown

	// Async dispatch so it never blocks HTTP request handling
	go func() {
		_, err := s.bot.Send(msg)
		if err != nil {
			s.logger.Error("failed to send telegram channel message",
				zap.String("channel_id", channelID),
				zap.Error(err),
			)
		} else {
			s.logger.Info("telegram channel message sent", zap.String("channel_id", channelID))
		}
	}()
}
