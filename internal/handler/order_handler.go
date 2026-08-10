package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
)

type OrderHandler struct {
	orderSvc *service.OrderService
	logger   *zap.Logger
}

func NewOrderHandler(orderSvc *service.OrderService, logger *zap.Logger) *OrderHandler {
	return &OrderHandler{
		orderSvc: orderSvc,
		logger:   logger,
	}
}

func (h *OrderHandler) RegisterRoutes(r *gin.RouterGroup) {
	orders := r.Group("/orders")
	{
		orders.GET("", h.List)
		orders.POST("", h.Create)
		orders.GET("/:id", h.GetByID)
	}
}

func (h *OrderHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	q := c.Query("q")
	status := c.Query("status")

	filter := repository.OrderFilter{
		Q:      q,
		Status: status,
		Page:   page,
		Limit:  limit,
	}

	orders, total, err := h.orderSvc.GetFilteredOrders(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to get orders", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if orders == nil {
		orders = []model.Order{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": orders,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

func (h *OrderHandler) Create(c *gin.Context) {
	var req service.CreateOrderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.orderSvc.CreateOrderHeaderWithItems(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("failed to create order", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "order created successfully",
		"data":    order,
	})
}

func (h *OrderHandler) GetByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	order, err := h.orderSvc.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": order,
	})
}
