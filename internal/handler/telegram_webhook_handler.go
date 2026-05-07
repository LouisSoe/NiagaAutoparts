package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"github.com/louissoe/niaga-autoparts/internal/worker"
	"go.uber.org/zap"
)

// TelegramWebhookHandler receives update events pushed by the Telegram Bot API.
// It converts each Telegram Update into the generic IncomingMessage and dispatches
// it to the shared worker pool — the same pool used by Fonnte.
type TelegramWebhookHandler struct {
	workerPool  *worker.Pool
	telegramSvc *service.TelegramService
	logger      *zap.Logger
}

// NewTelegramWebhookHandler constructs the handler.
func NewTelegramWebhookHandler(
	pool *worker.Pool,
	telegramSvc *service.TelegramService,
	logger *zap.Logger,
) *TelegramWebhookHandler {
	return &TelegramWebhookHandler{
		workerPool:  pool,
		telegramSvc: telegramSvc,
		logger:      logger,
	}
}

// RegisterRoutes attaches the Telegram webhook route to the router.
func (h *TelegramWebhookHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/telegram/webhook", h.HandleUpdate)
}

// HandleUpdate is called by Telegram for every incoming update (POST JSON body).
// It immediately returns HTTP 200 to prevent Telegram from retrying, then
// dispatches the job asynchronously to the shared worker pool.
func (h *TelegramWebhookHandler) HandleUpdate(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Warn("failed to read telegram update body", zap.Error(err))
		c.Status(http.StatusBadRequest)
		return
	}

	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		h.logger.Warn("failed to parse telegram update", zap.Error(err))
		c.Status(http.StatusBadRequest)
		return
	}

	// Convert Telegram update to generic IncomingMessage
	incoming, skip := h.telegramSvc.ProcessUpdate(update)
	if skip {
		c.Status(http.StatusOK) // acknowledge silently
		return
	}

	h.logger.Info("telegram webhook received",
		zap.String("sender", incoming.Sender),
		zap.Bool("has_attachment", incoming.AttachmentURL != ""),
	)

	job := model.WorkerJob{
		Payload:  incoming,
		RecvedAt: time.Now(),
	}
	if !h.workerPool.Dispatch(job) {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.telegramSvc.SendText(bgCtx, model.PlatformTelegram, incoming.Sender,
				"⏳ Sistem sedang sibuk. Pesan Anda akan diproses sesaat lagi.")
		}()
	}

	// Telegram requires a 200 OK within a few seconds; body is ignored.
	c.Status(http.StatusOK)
}
