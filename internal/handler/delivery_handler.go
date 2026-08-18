package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
)

// DeliveryHandler provides REST endpoints for delivery schedules and requests.
type DeliveryHandler struct {
	deliverySvc *service.DeliveryService
	logger      *zap.Logger
}

func NewDeliveryHandler(deliverySvc *service.DeliveryService, logger *zap.Logger) *DeliveryHandler {
	return &DeliveryHandler{
		deliverySvc: deliverySvc,
		logger:      logger,
	}
}

func (h *DeliveryHandler) RegisterRoutes(apiGroup *gin.RouterGroup) {
	deliveries := apiGroup.Group("/deliveries")
	{
		deliveries.GET("/", h.listDeliveries)
		deliveries.GET("/available-schedules", h.getAvailableSchedules)
		deliveries.GET("/estimate-shipping-cost", h.estimateShippingCost)
		deliveries.POST("/request", h.requestDelivery)
		deliveries.POST("/:id/approve", h.courierApprove)
		deliveries.POST("/:id/reschedule-suggest", h.courierSuggestReschedule)
		deliveries.POST("/:id/reschedule-accept", h.customerAcceptReschedule)
	}

	deliverySchedules := apiGroup.Group("/delivery-schedules")
	{
		deliverySchedules.GET("", h.listSchedules)
		deliverySchedules.GET("/:id", h.getScheduleByID)
		deliverySchedules.POST("", h.createSchedule)
		deliverySchedules.PUT("/:id", h.updateSchedule)
		deliverySchedules.DELETE("/:id", h.deleteSchedule)
	}
}

func (h *DeliveryHandler) listDeliveries(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format tanggal tidak valid (gunakan YYYY-MM-DD)"})
		return
	}

	deliveries, err := h.deliverySvc.GetDeliveriesForDate(c.Request.Context(), targetDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "date": dateStr, "data": deliveries})
}

func (h *DeliveryHandler) getAvailableSchedules(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format tanggal tidak valid (gunakan YYYY-MM-DD)"})
		return
	}

	schedules, err := h.deliverySvc.GetAvailableSchedules(c.Request.Context(), targetDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"date":    dateStr,
		"data":    schedules,
	})
}

func (h *DeliveryHandler) estimateShippingCost(c *gin.Context) {
	latStr := c.Query("latitude")
	if latStr == "" {
		latStr = c.Query("lat")
	}
	lngStr := c.Query("longitude")
	if lngStr == "" {
		lngStr = c.Query("lng")
	}

	if latStr == "" || lngStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Parameter latitude dan longitude wajib disertakan (contoh: ?latitude=-7.98&longitude=112.63)",
		})
		return
	}

	lat, err1 := strconv.ParseFloat(latStr, 64)
	lng, err2 := strconv.ParseFloat(lngStr, 64)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Format latitude atau longitude tidak valid",
		})
		return
	}

	res := h.deliverySvc.CalculateShippingCost(lat, lng)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    res,
	})
}

func (h *DeliveryHandler) requestDelivery(c *gin.Context) {
	var input service.RequestDeliveryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delivery, err := h.deliverySvc.RequestDelivery(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Permintaan pengantaran berhasil dibuat dan menunggu konfirmasi kurir",
		"data":    delivery,
	})
}

func (h *DeliveryHandler) courierApprove(c *gin.Context) {
	idStr := c.Param("id")
	deliveryID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID pengantaran tidak valid"})
		return
	}

	if err := h.deliverySvc.CourierApprove(c.Request.Context(), deliveryID, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Jadwal pengantaran berhasil dikonfirmasi",
	})
}

type suggestRescheduleInput struct {
	SuggestedDate       string `json:"suggested_date" binding:"required"`
	SuggestedScheduleID int64  `json:"suggested_schedule_id" binding:"required"`
	Reason              string `json:"reason"`
}

func (h *DeliveryHandler) courierSuggestReschedule(c *gin.Context) {
	idStr := c.Param("id")
	deliveryID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID pengantaran tidak valid"})
		return
	}

	var input suggestRescheduleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sugDate, err := time.Parse("2006-01-02", input.SuggestedDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format tanggal tidak valid (gunakan YYYY-MM-DD)"})
		return
	}

	if err := h.deliverySvc.CourierSuggestReschedule(c.Request.Context(), deliveryID, sugDate, input.SuggestedScheduleID, input.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Saran perubahan jadwal berhasil dikirimkan ke customer",
	})
}

func (h *DeliveryHandler) customerAcceptReschedule(c *gin.Context) {
	idStr := c.Param("id")
	deliveryID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID pengantaran tidak valid"})
		return
	}

	if err := h.deliverySvc.CustomerAcceptReschedule(c.Request.Context(), deliveryID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Jadwal baru berhasil diterima dan dikonfirmasi",
	})
}

// ─── Master Delivery Schedules CRUD Handlers ─────────────────────────────────

func (h *DeliveryHandler) listSchedules(c *gin.Context) {
	dayOfWeek := c.Query("day_of_week")
	var isActive *bool
	if activeStr := c.Query("is_active"); activeStr != "" {
		activeVal := (activeStr == "true" || activeStr == "1")
		isActive = &activeVal
	}

	schedules, err := h.deliverySvc.GetAllSchedules(c.Request.Context(), dayOfWeek, isActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    schedules,
	})
}

func (h *DeliveryHandler) getScheduleByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID jadwal tidak valid"})
		return
	}

	schedule, err := h.deliverySvc.GetScheduleByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Jadwal pengantaran tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    schedule,
	})
}

type scheduleRequestPayload struct {
	DayOfWeek   string `json:"day_of_week" binding:"required"`
	SlotName    string `json:"slot_name" binding:"required"`
	StartTime   string `json:"start_time" binding:"required"`
	EndTime     string `json:"end_time" binding:"required"`
	MaxCapacity int    `json:"max_capacity"`
	IsActive    *bool  `json:"is_active"`
}

func (h *DeliveryHandler) createSchedule(c *gin.Context) {
	var payload scheduleRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}

	maxCap := payload.MaxCapacity
	if maxCap <= 0 {
		maxCap = 5
	}

	schedule := model.DeliverySchedule{
		DayOfWeek:   payload.DayOfWeek,
		SlotName:    payload.SlotName,
		StartTime:   payload.StartTime,
		EndTime:     payload.EndTime,
		MaxCapacity: maxCap,
		IsActive:    isActive,
	}

	if err := h.deliverySvc.CreateSchedule(c.Request.Context(), &schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Master jadwal pengantaran berhasil dibuat",
		"data":    schedule,
	})
}

func (h *DeliveryHandler) updateSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID jadwal tidak valid"})
		return
	}

	var payload scheduleRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}

	maxCap := payload.MaxCapacity
	if maxCap <= 0 {
		maxCap = 5
	}

	schedule := model.DeliverySchedule{
		ID:          id,
		DayOfWeek:   payload.DayOfWeek,
		SlotName:    payload.SlotName,
		StartTime:   payload.StartTime,
		EndTime:     payload.EndTime,
		MaxCapacity: maxCap,
		IsActive:    isActive,
	}

	if err := h.deliverySvc.UpdateSchedule(c.Request.Context(), &schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Master jadwal pengantaran berhasil diperbarui",
		"data":    schedule,
	})
}

func (h *DeliveryHandler) deleteSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID jadwal tidak valid"})
		return
	}

	if err := h.deliverySvc.DeleteSchedule(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Master jadwal pengantaran berhasil dihapus",
	})
}

