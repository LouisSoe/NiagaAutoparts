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

type ProductHandler struct {
	svc    *service.ProductService
	logger *zap.Logger
}

func NewProductHandler(svc *service.ProductService, logger *zap.Logger) *ProductHandler {
	return &ProductHandler{
		svc:    svc,
		logger: logger,
	}
}

func (h *ProductHandler) RegisterRoutes(rg *gin.RouterGroup) {
	products := rg.Group("/products")
	{
		products.GET("", h.List)
		products.GET("/:id", h.GetByID)
		products.POST("", h.Create)
		products.PUT("/:id", h.Update)
		products.DELETE("/:id", h.Delete)
	}
}

func (h *ProductHandler) List(c *gin.Context) {
	q := c.Query("q")
	stockStatus := c.Query("stock_status")

	var categoryID *int64
	if catIDStr := c.Query("category_id"); catIDStr != "" {
		if id, err := strconv.ParseInt(catIDStr, 10, 64); err == nil {
			categoryID = &id
		}
	}

	var isActive *bool
	if activeStr := c.Query("is_active"); activeStr != "" {
		if b, err := strconv.ParseBool(activeStr); err == nil {
			isActive = &b
		}
	}

	var lowStockPriority *bool
	if lspStr := c.Query("low_stock_priority"); lspStr != "" {
		if b, err := strconv.ParseBool(lspStr); err == nil {
			lowStockPriority = &b
		}
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	filter := repository.ProductFilter{
		Q:                q,
		CategoryID:       categoryID,
		StockStatus:      stockStatus,
		IsActive:         isActive,
		LowStockPriority: lowStockPriority,
		Page:             page,
		Limit:            limit,
	}

	products, total, err := h.svc.GetFilteredProducts(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	c.JSON(http.StatusOK, model.PaginatedResponse{
		Data: products,
		Meta: model.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *ProductHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product ID"})
		return
	}

	product, refs, err := h.svc.GetWithPriceRefs(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":             product,
		"price_references": refs,
	})
}

func (h *ProductHandler) Create(c *gin.Context) {
	var p model.Product
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p.IsActive = true

	if err := h.svc.CreateProduct(c.Request.Context(), &p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "product created successfully", "data": p})
}

func (h *ProductHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product ID"})
		return
	}

	var p model.Product
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p.ID = id

	if err := h.svc.UpdateProduct(c.Request.Context(), &p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "product updated successfully", "data": p})
}

func (h *ProductHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product ID"})
		return
	}

	if err := h.svc.DeleteProduct(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "product deleted successfully"})
}
