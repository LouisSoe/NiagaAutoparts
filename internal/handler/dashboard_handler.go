package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
)

type DashboardHandler struct {
	svc    *service.DashboardService
	logger *zap.Logger
}

func NewDashboardHandler(svc *service.DashboardService, logger *zap.Logger) *DashboardHandler {
	return &DashboardHandler{
		svc:    svc,
		logger: logger,
	}
}

func (h *DashboardHandler) RegisterRoutes(rg *gin.RouterGroup) {
	dashboard := rg.Group("/dashboard")
	{
		dashboard.GET("", h.GetDashboard)
		dashboard.GET("/summary", h.GetDashboard)
	}
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	data, err := h.svc.GetDashboardData(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get dashboard data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve dashboard data"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
	})
}
