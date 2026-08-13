package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Logger returns a Gin middleware that logs each request using zap.
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		logger.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// CORS middleware handles Preflight (OPTIONS) requests and sets CORS headers.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Recovery returns a Gin middleware that catches panics and returns HTTP 500, sending alert to Telegram Error Channel.
func Recovery(logger *zap.Logger, notifierSvc *service.TelegramNotifierService) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("%v", r)
				logger.Error("panic recovered in http handler", zap.Any("panic", r))
				if notifierSvc != nil {
					ctxInfo := fmt.Sprintf("HTTP %s %s (IP: %s)", c.Request.Method, c.Request.URL.Path, c.ClientIP())
					notifierSvc.SendErrorAlert(c.Request.Context(), errMsg, ctxInfo)
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
		}()
		c.Next()
	}
}

// ─── Per-IP Rate Limiting ─────────────────────────────────────────────────────

// rateLimiter holds a limiter per sender/phone number.
// For webhook usage we key on the sender field, not IP.
// This is a simple in-process map; for multi-instance deployments use Redis.
type rateLimiterStore struct {
	limiters map[string]*rate.Limiter
	rps      float64
	burst    int
}

var store *rateLimiterStore

// InitRateLimiter initialises the global rate limiter store.
func InitRateLimiter(rps float64, burst int) {
	store = &rateLimiterStore{
		limiters: make(map[string]*rate.Limiter),
		rps:      rps,
		burst:    burst,
	}
}

// RateLimit returns a middleware that limits requests per sender phone number.
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract sender from form or query param (Fonnte sends as form)
		sender := c.PostForm("sender")
		if sender == "" {
			sender = c.ClientIP()
		}

		limiter := getLimiter(sender)
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests",
			})
			return
		}
		c.Next()
	}
}

func getLimiter(key string) *rate.Limiter {
	if store == nil {
		return rate.NewLimiter(rate.Inf, 1)
	}
	if l, ok := store.limiters[key]; ok {
		return l
	}
	l := rate.NewLimiter(rate.Limit(store.rps), store.burst)
	store.limiters[key] = l
	return l
}