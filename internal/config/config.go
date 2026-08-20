package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	App       AppConfig
	DB        DBConfig
	Fonnte    FonnteConfig
	Telegram  TelegramConfig
	Gemini    GeminiConfig
	Worker    WorkerConfig
	Session   SessionConfig
	Cache     CacheConfig
	RateLimit RateLimitConfig
	Midtrans  MidtransConfig
	GoogleMaps GoogleMapsConfig
}

type AppConfig struct {
	Port      string
	Env       string
	JWTSecret string
}

type DBConfig struct {
	Host string
	Port string
	User string
	Pass string
	Name string
	DSN  string
}

type FonnteConfig struct {
	Token  string
	APIURL string
}

type TelegramConfig struct {
	Token                  string // TELE_API — Bot 1 CS Token
	NotifierToken          string // TELEGRAM_NOTIFIER_BOT_TOKEN — Bot 2 Notifier Token
	CourierToken           string // TELEGRAM_COURIER_BOT_TOKEN — Bot 3 Courier Assistant Bot
	OrderChannelID         string // TELEGRAM_ORDER_CHANNEL_ID
	ErrorChannelID         string // TELEGRAM_ERROR_CHANNEL_ID
	CourierReminderMode    string // "daily" atau "interval" (default: "daily")
	CourierReminderHour    int    // Jam reminder harian (default: 5)
	CourierReminderMinute  int    // Menit reminder harian (default: 0)
	CourierReminderInterval int   // Interval reminder dalam menit (default: 60)
}

type GeminiConfig struct {
	APIKey  string
	Model   string
	Timeout time.Duration
}

type WorkerConfig struct {
	PoolSize  int
	QueueSize int
}

type SessionConfig struct {
	TTL time.Duration
}

// CacheConfig controls the built-in in-memory cache (no Redis required).
type CacheConfig struct {
	ProductTTL      time.Duration // How long product search results are cached
	CleanupInterval time.Duration // How often expired entries are swept from memory
}

type RateLimitConfig struct {
	PerSecond float64
	Burst     int
}

type GoogleMapsConfig struct {
	Enabled bool   // true if Google Maps integration is enabled
	ApiKey  string // Google Maps Distance Matrix API key
}

type MidtransConfig struct {
	MerchantID   string
	ServerKey    string
	ClientKey    string
	IsProduction bool
	SnapURL      string
	SnapJSURL    string
}

// Load reads configuration from the environment (after loading .env if present).
func Load() (*Config, error) {
	paths := []string{"../.env", "../../.env", ".env"}
	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			fmt.Printf("loaded .env from: %s\n", path)
			break
		}
	}

	cfg := &Config{}

	// App
	cfg.App.Port = getEnv("APP_PORT", "8080")
	cfg.App.Env = getEnv("APP_ENV", "development")
	cfg.App.JWTSecret = getEnv("JWT_SECRET", "autoparts_super_secret_jwt_key_2026")

	// Database (PostgreSQL)
	cfg.DB.Host = getEnv("DB_HOST", "localhost")
	cfg.DB.Port = getEnv("DB_PORT", "5432")
	cfg.DB.User = getEnv("DB_USER", "postgres")
	cfg.DB.Pass = getEnv("DB_PASS", "")
	cfg.DB.Name = getEnv("DB_NAME", "autoparts_db")
	cfg.DB.DSN = fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Jakarta",
		cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Pass, cfg.DB.Name,
	)

	// Fonnte
	cfg.Fonnte.Token = getEnv("FONNTE_TOKEN", "")
	cfg.Fonnte.APIURL = getEnv("FONNTE_API_URL", "https://api.fonnte.com/send")

	// Telegram
	cfg.Telegram.Token = getEnv("TELE_API", "")
	cfg.Telegram.NotifierToken = getEnv("TELEGRAM_NOTIFIER_BOT_TOKEN", "")
	cfg.Telegram.CourierToken = getEnv("TELEGRAM_COURIER_BOT_TOKEN", "")
	cfg.Telegram.OrderChannelID = getEnv("TELEGRAM_ORDER_CHANNEL_ID", "")
	cfg.Telegram.ErrorChannelID = getEnv("TELEGRAM_ERROR_CHANNEL_ID", "")
	cfg.Telegram.CourierReminderMode = strings.ToLower(getEnv("COURIER_REMINDER_MODE", "daily"))
	cfg.Telegram.CourierReminderHour = getEnvInt("COURIER_REMINDER_HOUR", 5)
	cfg.Telegram.CourierReminderMinute = getEnvInt("COURIER_REMINDER_MINUTE", 0)
	cfg.Telegram.CourierReminderInterval = getEnvInt("COURIER_REMINDER_INTERVAL_MINUTES", 60)

	// Gemini
	cfg.Gemini.APIKey = getEnv("GEMINI_API_KEY", "")
	cfg.Gemini.Model = getEnv("GEMINI_MODEL", "gemini-1.5-flash-latest")
	cfg.Gemini.Timeout = time.Duration(getEnvInt("GEMINI_TIMEOUT_SEC", 15)) * time.Second

	// Worker
	cfg.Worker.PoolSize = getEnvInt("WORKER_POOL_SIZE", 10)
	cfg.Worker.QueueSize = getEnvInt("WORKER_QUEUE_SIZE", 100)

	// Session
	cfg.Session.TTL = time.Duration(getEnvInt("SESSION_TTL_MINUTES", 30)) * time.Minute

	// In-memory cache
	cfg.Cache.ProductTTL = time.Duration(getEnvInt("CACHE_PRODUCT_TTL_SECONDS", 60)) * time.Second
	cfg.Cache.CleanupInterval = time.Duration(getEnvInt("CACHE_CLEANUP_INTERVAL_SECONDS", 120)) * time.Second

	// Rate Limiting
	cfg.RateLimit.PerSecond = getEnvFloat("RATE_LIMIT_PER_SECOND", 5.0)
	cfg.RateLimit.Burst = getEnvInt("RATE_LIMIT_BURST", 10)

	// Midtrans
	cfg.Midtrans.MerchantID = getEnv("MIDTRANS_MERCHANT_ID", "")
	cfg.Midtrans.IsProduction = getEnv("MIDTRANS_IS_PRODUCTION", "false") == "true"
	// Google Maps Distance Matrix configuration
	cfg.GoogleMaps.Enabled = getEnv("GOOGLE_MAPS_ENABLED", "false") == "true"
	cfg.GoogleMaps.ApiKey = getEnv("GOOGLE_MAPS_API_KEY", "")
	cfg.Midtrans.ServerKey = getEnv("MIDTRANS_SERVER_KEY", "")
	cfg.Midtrans.ClientKey = getEnv("MIDTRANS_CLIENT_KEY", "")

	defaultSnapURL := "https://app.sandbox.midtrans.com/snap/v1/transactions"
	defaultSnapJSURL := "https://app.sandbox.midtrans.com/snap/snap.js"
	if cfg.Midtrans.IsProduction {
		defaultSnapURL = "https://app.midtrans.com/snap/v1/transactions"
		defaultSnapJSURL = "https://app.midtrans.com/snap/snap.js"
	}

	cfg.Midtrans.SnapURL = defaultSnapURL
	cfg.Midtrans.SnapJSURL = getEnv("MIDTRANS_SNAP_URL", defaultSnapJSURL)

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return fallback
}