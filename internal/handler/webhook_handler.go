package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"github.com/louissoe/niaga-autoparts/internal/worker"
	"go.uber.org/zap"
)

// WebhookHandler receives inbound WhatsApp messages from Fonnte.
type WebhookHandler struct {
	workerPool   *worker.Pool
	messagingSvc *service.MessagingService
	logger       *zap.Logger
}

// NewWebhookHandler constructs the handler.
func NewWebhookHandler(
	pool *worker.Pool,
	messagingSvc *service.MessagingService,
	logger *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		workerPool:   pool,
		messagingSvc: messagingSvc,
		logger:       logger,
	}
}

// RegisterRoutes attaches all Fonnte-related routes to the Gin engine.
func (h *WebhookHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/webhook", h.HandleIncoming)
	r.GET("/webhook", h.VerifyWebhook)
	r.GET("/health", h.Health)
	r.GET("/metrics", h.Metrics)
	r.GET("/fonnte/test", h.TestFonnte)
}

// HandleIncoming is the main webhook endpoint called by Fonnte on every message.
// It immediately returns HTTP 200 to prevent Fonnte from retrying, then
// dispatches the job asynchronously to the worker pool.
func (h *WebhookHandler) HandleIncoming(c *gin.Context) {
	var payload model.FonnteWebhookPayload

	if err := c.ShouldBind(&payload); err != nil {
		h.logger.Warn("invalid webhook payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Ignore group messages and messages with no sender
	if payload.IsGroupMessage() || payload.Sender == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	h.logger.Info("fonnte webhook received",
		zap.String("sender", maskPhoneHandler(payload.Sender)),
		zap.Bool("has_image", payload.IsImageMessage()),
	)

	// Convert to generic IncomingMessage and dispatch
	job := model.WorkerJob{
		Payload:  payload.ToIncomingMessage(),
		RecvedAt: time.Now(),
	}
	if !h.workerPool.Dispatch(job) {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.messagingSvc.SendText(bgCtx, model.PlatformFonnte, payload.Sender,
				"⏳ Sistem sedang sibuk. Pesan Anda akan diproses sesaat lagi.")
		}()
	}

	c.JSON(http.StatusOK, gin.H{"status": "queued"})
}

// VerifyWebhook handles GET requests from Fonnte to verify the webhook URL.
func (h *WebhookHandler) VerifyWebhook(c *gin.Context) {
	h.logger.Info("webhook verification request received from Fonnte")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Health returns a simple liveness probe.
func (h *WebhookHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Metrics returns basic worker pool stats.
func (h *WebhookHandler) Metrics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"queue_length": h.workerPool.QueueLen(),
	})
}

// TestFonnte is a developer-only connectivity check.
// It sends a WhatsApp message to the number in the "target" query param.
//
// Usage:
//
//	GET /fonnte/test?target=6281234567890
func (h *WebhookHandler) TestFonnte(c *gin.Context) {
	target := c.Query("target")
	if target == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query param 'target' wajib diisi, contoh: ?target=6281234567890",
		})
		return
	}
	message := c.DefaultQuery("message", "Halo! Ini adalah pesan uji coba koneksi Fonnte dari NiagaGudang. ✅")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
	defer cancel()

	h.logger.Info("fonnte connectivity test", zap.String("target", maskPhoneHandler(target)))

	result, err := h.messagingSvc.Ping(ctx, target, message)
	if err != nil {
		h.logger.Error("fonnte ping failed", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}

	statusCode := http.StatusOK
	if !result.Status {
		statusCode = http.StatusBadGateway
	}
	c.JSON(statusCode, gin.H{
		"ok":      result.Status,
		"detail":  result.Detail,
		"id":      result.ID,
		"target":  result.Target,
		"process": result.Process,
	})
}

// maskPhoneHandler returns a privacy-safe phone string for logging.
func maskPhoneHandler(phone string) string {
	if len(phone) < 6 {
		return "****"
	}
	return phone[:6] + "****" + phone[len(phone)-2:]
}