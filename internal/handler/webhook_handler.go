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

// RegisterRoutes attaches all routes to the given Gin engine.
func (h *WebhookHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/webhook", h.HandleIncoming)
	r.GET("/webhook", h.VerifyWebhook) // Fonnte requires GET to verify the URL
	r.GET("/health", h.Health)
	r.GET("/metrics", h.Metrics)
	r.GET("/fonnte/test", h.TestFonnte) // Connectivity check — send a test WhatsApp message
}

// HandleIncoming is the main webhook endpoint called by Fonnte on every message.
//
// Two-step response strategy:
//  1. [DISABLED] Send an instant "processing..." ACK (<1 second) — dinonaktifkan
//  2. Dispatch the job to the worker pool for async processing
//  3. Return HTTP 200 immediately so Fonnte doesn't retry
func (h *WebhookHandler) HandleIncoming(c *gin.Context) {
	var payload model.FonnteWebhookPayload

	// Fonnte sends data as form values
	if err := c.ShouldBind(&payload); err != nil {
		h.logger.Warn("invalid webhook payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Ignore group messages (handle only personal chat)
	if payload.IsGroupMessage() {
		c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "group_message"})
		return
	}

	// Ignore empty messages
	if payload.Sender == "" {
		c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "no_sender"})
		return
	}

	h.logger.Info("webhook received",
		zap.String("sender", maskPhone(payload.Sender)),
		zap.Bool("has_image", payload.IsImageMessage()),
	)

	// Step 1: Send immediate acknowledgement (<1 second)
	// DISABLED — Balasan "sedang diproses" dinonaktifkan; hapus komentar di bawah untuk mengaktifkan kembali.
	// go func() {
	// 	// Use a fresh context for background tasks to avoid "context canceled"
	// 	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 	defer cancel()
	//
	// 	if err := h.messagingSvc.SendTyping(bgCtx, payload.Sender); err != nil {
	// 		h.logger.Warn("failed to send typing ack", zap.Error(err))
	// 	}
	// }()

	// Step 2: Dispatch to async worker pool
	job := model.WorkerJob{
		Payload:  payload,
		RecvedAt: time.Now(),
	}
	if !h.workerPool.Dispatch(job) {
		// Queue is full — send a polite overflow message
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_ = h.messagingSvc.SendText(bgCtx, payload.Sender,
				"⏳ Sistem sedang sibuk. Pesan Anda akan diproses sesaat lagi.")
		}()
	}

	// Step 3: Return 200 immediately so Fonnte doesn't retry
	c.JSON(http.StatusOK, gin.H{"status": "queued"})
}

// VerifyWebhook handles GET requests from Fonnte to verify the webhook URL.
// Fonnte sends a GET request before activating the webhook; we must respond 200.
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
//
// The response body is the raw JSON from Fonnte, identical to what the PHP
// example printed via echo $response.
func (h *WebhookHandler) TestFonnte(c *gin.Context) {
	target := c.Query("target")
	if target == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query param 'target' (nomor WhatsApp tujuan) wajib diisi, contoh: ?target=6281234567890",
		})
		return
	}

	message := c.DefaultQuery("message", "Halo! Ini adalah pesan uji coba koneksi Fonnte dari NiagaGudang. ✅")

	// Use 35s — slightly longer than pingClient's 30s hard timeout — so the
	// client timeout fires first and returns the descriptive network error
	// rather than a generic context cancellation.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
	defer cancel()

	h.logger.Info("fonnte connectivity test",
		zap.String("target", maskPhone(target)),
	)

	result, err := h.messagingSvc.Ping(ctx, target, message)
	if err != nil {
		h.logger.Error("fonnte ping failed", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
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

// maskPhone is duplicated here to avoid cross-package import just for logging.
func maskPhone(phone string) string {
	if len(phone) < 6 {
		return "****"
	}
	return phone[:6] + "****" + phone[len(phone)-2:]
}