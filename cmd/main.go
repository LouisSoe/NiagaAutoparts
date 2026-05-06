package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/louissoe/niaga-autoparts/internal/ai"
	"github.com/louissoe/niaga-autoparts/internal/cache"
	"github.com/louissoe/niaga-autoparts/internal/config"
	"github.com/louissoe/niaga-autoparts/internal/handler"
	"github.com/louissoe/niaga-autoparts/internal/middleware"
	"github.com/louissoe/niaga-autoparts/internal/repository"

	// "github.com/louissoe/niaga-autoparts/internal/scraper"
	"github.com/louissoe/niaga-autoparts/internal/service"
	"github.com/louissoe/niaga-autoparts/internal/worker"
)

func main() {
	// ─── Logger ───────────────────────────────────────────────────────────────
	logger := buildLogger()
	defer logger.Sync() //nolint:errcheck

	// ─── Config ───────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}
	logger.Info("configuration loaded", zap.String("env", cfg.App.Env))

	// ─── Database ─────────────────────────────────────────────────────────────
	db, err := connectDB(cfg.DB.DSN)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()
	logger.Info("database connected")

	// ─── In-Memory Cache (zero external dependencies) ─────────────────────────
	// Starts a background eviction goroutine; stopped on shutdown via defer.
	memCache := cache.New(cfg.Cache.CleanupInterval)
	defer memCache.Stop()
	logger.Info("in-memory cache started",
		zap.Duration("product_ttl", cfg.Cache.ProductTTL),
		zap.Duration("cleanup_interval", cfg.Cache.CleanupInterval),
	)

	// ─── Repositories ─────────────────────────────────────────────────────────
	productRepo := repository.NewProductRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	sessionRepo := repository.NewSessionRepository(db, cfg.Session.TTL)
	// priceRepo  := repository.NewPriceReferenceRepository(db)

	// ─── AI Service ───────────────────────────────────────────────────────────
	aiSvc := ai.NewAIService(
		cfg.Gemini.APIKey,
		cfg.Gemini.Model,
		cfg.Gemini.Timeout,
		logger,
	)

	// ─── Business Services ────────────────────────────────────────────────────
	intentSvc := service.NewIntentService(aiSvc, logger)
	productSvc := service.NewProductService(productRepo, memCache, cfg.Cache.ProductTTL, logger)
	orderSvc := service.NewOrderService(orderRepo, productRepo, logger)
	messagingSvc := service.NewMessagingService(cfg.Fonnte.Token, cfg.Fonnte.APIURL, logger)
	// priceCron, err  := scraper.NewPriceCronJob(productRepo, priceRepo, logger)
	if err := productSvc.BuildDictionary(context.Background()); err != nil {
		logger.Fatal("failed to build search dictionary", zap.Error(err))
	}

	processor := service.NewMessageProcessor(
		intentSvc,
		productSvc,
		orderSvc,
		sessionRepo,
		messagingSvc,
		aiSvc,
		cfg.Session.TTL,
		logger,
	)


	// ─── Worker Pool ──────────────────────────────────────────────────────────
	pool := worker.NewPool(
		cfg.Worker.PoolSize,
		cfg.Worker.QueueSize,
		processor,
		messagingSvc,
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ─── Start background services ────────────────────────────────────────────────
	// if err != nil {
	// 	logger.Warn("price scraper unavailable — skipping", zap.Error(err))
	// } else {
	// 	priceCron.Start(ctx)
	// }

	pool.Start(ctx)
	

	// Background ticker: expire old reservations every minute
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				orderSvc.ExpireOldReservations(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	// ─── HTTP Server ──────────────────────────────────────────────────────────
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	middleware.InitRateLimiter(cfg.RateLimit.PerSecond, cfg.RateLimit.Burst)

	router := gin.New()
	router.Use(
		middleware.Recovery(logger),
		middleware.Logger(logger),
		middleware.RateLimit(),
	)

	webhookHandler := handler.NewWebhookHandler(pool, messagingSvc, logger)
	webhookHandler.RegisterRoutes(router)

	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", zap.String("port", cfg.App.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// ─── Graceful Shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}

	pool.Stop()
	logger.Info("server exited cleanly")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func connectDB(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func buildLogger() *zap.Logger {
	env := os.Getenv("APP_ENV")

	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := cfg.Build()
	if err != nil {
		panic("failed to build logger: " + err.Error())
	}
	return logger
}

var _ = sql.ErrNoRows