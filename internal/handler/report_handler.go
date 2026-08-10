package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
)

type ReportHandler struct {
	svc    *service.ReportService
	logger *zap.Logger
}

func NewReportHandler(svc *service.ReportService, logger *zap.Logger) *ReportHandler {
	return &ReportHandler{
		svc:    svc,
		logger: logger,
	}
}

func (h *ReportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	reports := rg.Group("/reports")
	{
		reports.GET("/sales", h.GetSalesReport)
		reports.GET("/sales/export", h.ExportSalesReportCSV)
		reports.GET("/sales/export/pdf", h.ExportSalesReportPDF)
		reports.GET("/sales/export/excel", h.ExportSalesReportExcel)
		reports.GET("/sales/export/xlsx", h.ExportSalesReportExcel)

		reports.GET("/export/pdf", h.ExportSalesReportPDF)
		reports.GET("/export/excel", h.ExportSalesReportExcel)
		reports.GET("/export/xlsx", h.ExportSalesReportExcel)

		reports.GET("/stock/export", h.ExportStockReportCSV)
		reports.GET("/stock/export/pdf", h.ExportStockReportPDF)
		reports.GET("/stock/export/excel", h.ExportStockReportExcel)
		reports.GET("/stock/export/xlsx", h.ExportStockReportExcel)
	}
}

func (h *ReportHandler) GetSalesReport(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	filter := model.SalesReportFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Status:    c.Query("status"),
		Page:      page,
		Limit:     limit,
	}

	data, total, err := h.svc.GetSalesReport(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to get sales report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve sales report"})
		return
	}

	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"meta": model.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *ReportHandler) ExportSalesReportCSV(c *gin.Context) {
	filter := model.SalesReportFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Status:    c.Query("status"),
	}

	filename := fmt.Sprintf("laporan_penjualan_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.svc.ExportSalesReportCSV(c.Request.Context(), c.Writer, filter); err != nil {
		h.logger.Error("failed to export sales report CSV", zap.Error(err))
	}
}

func (h *ReportHandler) ExportSalesReportPDF(c *gin.Context) {
	filter := model.SalesReportFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Status:    c.Query("status"),
	}

	filename := fmt.Sprintf("laporan_penjualan_%s.pdf", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.svc.ExportSalesReportPDF(c.Request.Context(), c.Writer, filter); err != nil {
		h.logger.Error("failed to export sales report PDF", zap.Error(err))
	}
}

func (h *ReportHandler) ExportStockReportCSV(c *gin.Context) {
	categoryID, _ := strconv.ParseInt(c.Query("category_id"), 10, 64)
	lowStockOnly := c.Query("low_stock_only") == "true"

	filename := fmt.Sprintf("laporan_stok_gudang_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.svc.ExportStockReportCSV(c.Request.Context(), c.Writer, categoryID, lowStockOnly); err != nil {
		h.logger.Error("failed to export stock report CSV", zap.Error(err))
	}
}

func (h *ReportHandler) ExportStockReportPDF(c *gin.Context) {
	categoryID, _ := strconv.ParseInt(c.Query("category_id"), 10, 64)
	lowStockOnly := c.Query("low_stock_only") == "true"

	filename := fmt.Sprintf("laporan_stok_gudang_%s.pdf", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.svc.ExportStockReportPDF(c.Request.Context(), c.Writer, categoryID, lowStockOnly); err != nil {
		h.logger.Error("failed to export stock report PDF", zap.Error(err))
	}
}

func (h *ReportHandler) ExportSalesReportExcel(c *gin.Context) {
	filter := model.SalesReportFilter{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Status:    c.Query("status"),
	}

	filename := fmt.Sprintf("laporan_penjualan_%s.xlsx", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.svc.ExportSalesReportExcel(c.Request.Context(), c.Writer, filter); err != nil {
		h.logger.Error("failed to export sales report Excel", zap.Error(err))
	}
}

func (h *ReportHandler) ExportStockReportExcel(c *gin.Context) {
	categoryID, _ := strconv.ParseInt(c.Query("category_id"), 10, 64)
	lowStockOnly := c.Query("low_stock_only") == "true"

	filename := fmt.Sprintf("laporan_stok_gudang_%s.xlsx", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.svc.ExportStockReportExcel(c.Request.Context(), c.Writer, categoryID, lowStockOnly); err != nil {
		h.logger.Error("failed to export stock report Excel", zap.Error(err))
	}
}


