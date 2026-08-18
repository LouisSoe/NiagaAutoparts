package main

import (
	"context"
	"database/sql"
	"fmt"
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
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/louissoe/niaga-autoparts/internal/ai"
	"github.com/louissoe/niaga-autoparts/internal/cache"
	"github.com/louissoe/niaga-autoparts/internal/config"
	"github.com/louissoe/niaga-autoparts/internal/handler"
	"github.com/louissoe/niaga-autoparts/internal/middleware"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
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

	// ─── Auto Migration (Disabled) ────────────────────────────────────────────
	// if err := runMigrations(db.DB, logger); err != nil {
	// 	logger.Fatal("migration failed", zap.Error(err))
	// }

	// ─── In-Memory Cache ──────────────────────────────────────────────────────
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
	categoryRepo := repository.NewCategoryRepository(db)
	userRepo := repository.NewUserRepository(db)
	customerRepo := repository.NewCustomerRepository(db)
	dashboardRepo := repository.NewDashboardRepository(db)
	reportRepo := repository.NewReportRepository(db)
	deliveryRepo := repository.NewDeliveryRepository(db)
	deliveryScheduleRepo := repository.NewDeliveryScheduleRepository(db)

	// ─── AI Service ───────────────────────────────────────────────────────────
	aiSvc := ai.NewAIService(
		cfg.Gemini.APIKey,
		cfg.Gemini.Model,
		cfg.Gemini.Timeout,
		logger,
	)

	// ─── Fonnte Service ───────────────────────────────────────────────────────
	fonnteSvc := service.NewMessagingService(cfg.Fonnte.Token, cfg.Fonnte.APIURL, logger)

	// ─── Telegram Service ─────────────────────────────────────────────────────
	var telegramSvc *service.TelegramService
	if cfg.Telegram.Token != "" {
		telegramSvc, err = service.NewTelegramService(cfg.Telegram.Token, logger)
		if err != nil {
			logger.Warn("telegram service init failed — telegram will be disabled", zap.Error(err))
			telegramSvc = nil
		}
	} else {
		logger.Warn("TELE_API not set — telegram bot disabled")
	}

	// ─── Composite MessageSender ──────────────────────────────────────────────
	// Routes each outgoing message to the correct provider using the Platform
	// field that was stamped on the IncomingMessage at ingest time.
	var compositeSender model.MessageSender
	if telegramSvc != nil {
		compositeSender = &compositeMessageSender{fonnte: fonnteSvc, telegram: telegramSvc}
	} else {
		compositeSender = fonnteSvc
	}

	// ─── Business Services ────────────────────────────────────────────────────
	intentSvc := service.NewIntentService(aiSvc, logger)
	productSvc := service.NewProductService(productRepo, memCache, cfg.Cache.ProductTTL, logger)
	orderSvc := service.NewOrderService(orderRepo, productRepo, logger)
	categorySvc := service.NewCategoryService(categoryRepo, logger)
	userSvc := service.NewUserService(userRepo, logger, cfg.App.JWTSecret)
	userSvc.SetCustomerRepository(customerRepo)
	customerSvc := service.NewCustomerService(customerRepo, logger)
	midtransSvc := service.NewMidtransService(cfg.Midtrans, orderSvc, customerRepo, logger)
	dashboardSvc := service.NewDashboardService(dashboardRepo, logger)
	reportSvc := service.NewReportService(reportRepo, logger)
	deliverySvc := service.NewDeliveryService(deliveryRepo, deliveryScheduleRepo, customerRepo, orderRepo, logger)
	deliverySvc.SetMessageSender(compositeSender)

	// Initialize Google Maps Service if enabled
	if cfg.GoogleMaps.Enabled {
		mapsSvc := service.NewGoogleMapsService(cfg.GoogleMaps.ApiKey)
		deliverySvc.SetGoogleMapsService(mapsSvc)
	}

	notifierSvc, err := service.NewTelegramNotifierService(
		cfg.Telegram.NotifierToken,
		cfg.Telegram.OrderChannelID,
		cfg.Telegram.ErrorChannelID,
		logger,
	)
	if err != nil {
		logger.Error("failed to initialize telegram notifier service", zap.Error(err))
	} else if notifierSvc != nil {
		orderSvc.SetNotifierService(notifierSvc)
	}

	if err := productSvc.BuildDictionary(context.Background()); err != nil {
		logger.Fatal("failed to build search dictionary", zap.Error(err))
	}

	processor := service.NewMessageProcessor(
		intentSvc,
		productSvc,
		orderSvc,
		sessionRepo,
		compositeSender,
		aiSvc,
		cfg.Session.TTL,
		logger,
	)

	midtransSvc.SetMessageSender(compositeSender)
	orderSvc.SetUserRepository(userRepo)
	orderSvc.SetCustomerRepository(customerRepo)
	orderSvc.SetMessageSender(compositeSender)
	orderSvc.SetMidtransService(midtransSvc)
	processor.SetMidtransService(midtransSvc)
	processor.SetReportService(reportSvc)
	processor.SetDeliveryService(deliverySvc)
	processor.SetCustomerRepository(customerRepo)
	processor.SetUserRepository(userRepo)

	// ─── Worker Pool ──────────────────────────────────────────────────────────
	pool := worker.NewPool(
		cfg.Worker.PoolSize,
		cfg.Worker.QueueSize,
		processor,
		compositeSender,
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		middleware.CORS(),
		middleware.Recovery(logger, notifierSvc),
		middleware.Logger(logger),
		middleware.RateLimit(),
	)

	// Static file server untuk foto produk
	router.Static("/uploads", "./uploads")

	// Courier Map Web View routes
	courierMapHandler := handler.NewCourierMapHandler(logger)
	courierMapHandler.RegisterPublicRoutes(router)

	// Courier Bot 3 (Telegram Delivery Assistant)
	if cfg.Telegram.CourierToken != "" {
		webAppBase := os.Getenv("APP_BASE_URL") // jika menggunakan ngrok atau domain publik (misal https://xyz.ngrok-free.app)
		courierBotSvc, err := service.NewCourierBotService(cfg.Telegram.CourierToken, webAppBase, logger)
		if err != nil {
			logger.Error("failed to initialize courier bot service", zap.Error(err))
		} else {
			courierBotSvc.SetDeliveryService(deliverySvc)
			deliverySvc.SetCourierBot(courierBotSvc)
			courierBotSvc.StartPolling(ctx)

			// Background scheduler: Kirim Daily Reminder Pengantaran setiap hari pk 05:00 WIB
			go func() {
				loc, errLoc := time.LoadLocation("Asia/Jakarta")
				if errLoc != nil {
					loc = time.Local
				}

				for {
					now := time.Now().In(loc)
					nextRun := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, loc)
					if now.After(nextRun) {
						nextRun = nextRun.Add(24 * time.Hour)
					}

					durationUntilNextRun := time.Until(nextRun)
					logger.Info("courier morning digest scheduled", zap.Time("next_run", nextRun), zap.Duration("wait", durationUntilNextRun))

					select {
					case <-time.After(durationUntilNextRun):
						courierBotSvc.SendDailyMorningDigest(ctx)
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	} else {
		logger.Warn("TELEGRAM_COURIER_BOT_TOKEN not set — courier bot disabled")
	}

	// Fonnte (WhatsApp) routes
	fonnteHandler := handler.NewWebhookHandler(pool, fonnteSvc, logger)
	fonnteHandler.RegisterRoutes(router)

	// Telegram routes (only when token is configured)
	if telegramSvc != nil {
		tgHandler := handler.NewTelegramWebhookHandler(pool, telegramSvc, logger)
		tgHandler.RegisterRoutes(router)
	}

	// ─── REST API v1 Routes (CRUD for Product, Category, User, Customer) ────
	apiV1 := router.Group("/api/v1")
	apiV1.Use(middleware.AuthMiddleware(cfg.App.JWTSecret))
	{
		productHandler := handler.NewProductHandler(productSvc, logger)
		productHandler.RegisterRoutes(apiV1)

		categoryHandler := handler.NewCategoryHandler(categorySvc, logger)
		categoryHandler.RegisterRoutes(apiV1)

		userHandler := handler.NewUserHandler(userSvc, logger)
		userHandler.RegisterRoutes(apiV1)

		customerHandler := handler.NewCustomerHandler(customerSvc, logger)
		customerHandler.RegisterRoutes(apiV1)

		orderHandler := handler.NewOrderHandler(orderSvc, logger)
		orderHandler.RegisterRoutes(apiV1)

		paymentHandler := handler.NewPaymentHandler(midtransSvc, logger)
		paymentHandler.RegisterRoutes(apiV1, router)

		dashboardHandler := handler.NewDashboardHandler(dashboardSvc, logger)
		dashboardHandler.RegisterRoutes(apiV1)

		reportHandler := handler.NewReportHandler(reportSvc, logger)
		reportHandler.RegisterRoutes(apiV1)

		deliveryHandler := handler.NewDeliveryHandler(deliverySvc, logger)
		deliveryHandler.RegisterRoutes(apiV1)
	}

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

// ─── compositeMessageSender ───────────────────────────────────────────────────

// compositeMessageSender routes outgoing messages to the correct provider
// based on the explicit Platform stamped on the IncomingMessage at ingest time.
// This completely removes any need for heuristics over the "to" address format.
type compositeMessageSender struct {
	fonnte   model.MessageSender
	telegram model.MessageSender
}

func (c *compositeMessageSender) SendText(ctx context.Context, platform model.Platform, to, message string) error {
	if platform == model.PlatformTelegram && c.telegram != nil {
		return c.telegram.SendText(ctx, platform, to, message)
	}
	return c.fonnte.SendText(ctx, platform, to, message)
}

func (c *compositeMessageSender) SendMedia(ctx context.Context, platform model.Platform, to, caption, mediaURL, filename, mimeType string) error {
	if platform == model.PlatformTelegram && c.telegram != nil {
		return c.telegram.SendMedia(ctx, platform, to, caption, mediaURL, filename, mimeType)
	}
	return c.fonnte.SendMedia(ctx, platform, to, caption, mediaURL, filename, mimeType)
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

	// Lumberjack log rotation (7 days retention)
	lumberjackLogger := &lumberjack.Logger{
		Filename:   "logs/app.log",
		MaxSize:    10,   // megabytes
		MaxBackups: 14,   // max files
		MaxAge:     7,    // 7 days retention
		Compress:   true, // compress with gzip
	}

	var encoder zapcore.Encoder
	if env == "production" {
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.TimeKey = "ts"
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encCfg := zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zap.DebugLevel),
		zapcore.NewCore(encoder, zapcore.AddSync(lumberjackLogger), zap.DebugLevel),
	)

	return zap.New(core)
}

func runMigrations(db *sql.DB, logger *zap.Logger) error {
	migrationFiles := []string{
		"migrations/001_init.sql",
		"migrations/002_add_categories_users_customers.sql",
		"migrations/003_drop_category_column_from_products.sql",
		"migrations/004_refactor_order_header_detail.sql",
		"migrations/005_rename_price_to_purchase_price_and_add_selling_price_minimum_stock.sql",
		"migrations/006_update_users_role_check_constraint.sql",
		"migrations/007_refactor_users_and_customers.sql",
		"migrations/008_add_type_customer_to_customers.sql",
		"migrations/009_refactor_orders_payment_fields.sql",
		"migrations/010_remove_product_name_from_order_details.sql",
		"migrations/011_remove_user_id_from_orders.sql",
		"migrations/012_use_user_id_drop_customer_id_from_orders.sql",
		"migrations/018_create_delivery_schedules_and_deliveries.sql",
	}

	for _, file := range migrationFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			logger.Warn("migration file skip/not found", zap.String("file", file), zap.Error(err))
			continue
		}
		if _, err := db.Exec(string(content)); err != nil {
			logger.Error("migration failed", zap.String("file", file), zap.Error(err))
			return fmt.Errorf("migration %s failed: %w", file, err)
		}
		logger.Info("migration executed successfully", zap.String("file", file))
	}
	return nil
}
