package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
)

type PaymentHandler struct {
	midtransSvc *service.MidtransService
	logger      *zap.Logger
}

func NewPaymentHandler(midtransSvc *service.MidtransService, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{
		midtransSvc: midtransSvc,
		logger:      logger,
	}
}

func (h *PaymentHandler) RegisterRoutes(rg *gin.RouterGroup, router *gin.Engine) {
	payments := rg.Group("/payments")
	{
		payments.GET("/config", h.GetConfig)
		payments.POST("/snap-token", h.CreateSnapToken)
		payments.POST("/midtrans-notification", h.HandleNotification)
	}

	// Direct webhook endpoint alias
	router.POST("/webhook/midtrans", h.HandleNotification)
}

type CreateSnapTokenRequest struct {
	OrderID int64 `json:"order_id" binding:"required"`
}

func (h *PaymentHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"merchant_id":   h.midtransSvc.GetMerchantID(),
		"client_key":    h.midtransSvc.GetClientKey(),
		"is_production": h.midtransSvc.IsProduction(),
		"snap_url":      h.midtransSvc.GetSnapJSURL(),
	})
}

func (h *PaymentHandler) CreateSnapToken(c *gin.Context) {
	var req CreateSnapTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_id wajib diisi"})
		return
	}

	snapResp, err := h.midtransSvc.CreateSnapTransaction(c.Request.Context(), req.OrderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Token Midtrans Snap berhasil dibuat",
		"data":    snapResp,
	})
}

func (h *PaymentHandler) HandleNotification(c *gin.Context) {
	var notif service.MidtransNotification
	if err := c.ShouldBindJSON(&notif); err != nil {
		// Selalu return 200 agar Midtrans tidak retry — error cukup di-log
		h.logger.Warn("invalid Midtrans notification payload", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
		return
	}

	if err := h.midtransSvc.ProcessNotification(c.Request.Context(), notif); err != nil {
		// Selalu return 200 agar Midtrans tidak retry — error cukup di-log
		h.logger.Error("failed to process Midtrans notification", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "OK", "message": "Notification processed successfully"})
}
