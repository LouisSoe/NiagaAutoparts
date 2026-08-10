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

type CustomerHandler struct {
	svc    *service.CustomerService
	logger *zap.Logger
}

func NewCustomerHandler(svc *service.CustomerService, logger *zap.Logger) *CustomerHandler {
	return &CustomerHandler{
		svc:    svc,
		logger: logger,
	}
}

func (h *CustomerHandler) RegisterRoutes(rg *gin.RouterGroup) {
	customers := rg.Group("/customers")
	{
		customers.GET("", h.List)
		customers.GET("/:id", h.GetByID)
		customers.POST("", h.Create)
		customers.PUT("/:id", h.Update)
		customers.DELETE("/:id", h.Delete)
	}
}

func (h *CustomerHandler) List(c *gin.Context) {
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	filter := repository.CustomerFilter{
		Q:     q,
		Page:  page,
		Limit: limit,
	}

	customers, total, err := h.svc.GetFilteredCustomers(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	c.JSON(http.StatusOK, model.PaginatedResponse{
		Data: customers,
		Meta: model.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *CustomerHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer ID"})
		return
	}

	cust, err := h.svc.GetCustomerByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "customer not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cust})
}

func (h *CustomerHandler) Create(c *gin.Context) {
	var input service.CreateCustomerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cust, err := h.svc.CreateCustomer(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "customer created successfully", "data": cust})
}

func (h *CustomerHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer ID"})
		return
	}

	var input service.UpdateCustomerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cust, err := h.svc.UpdateCustomer(c.Request.Context(), id, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "customer updated successfully", "data": cust})
}

func (h *CustomerHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customer ID"})
		return
	}

	if err := h.svc.DeleteCustomer(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "customer deleted successfully"})
}
