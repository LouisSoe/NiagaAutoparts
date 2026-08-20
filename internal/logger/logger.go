package logger

import (
	"io"
	"os"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
)

// NewLogger creates a new logrus.Logger configured with rotatelogs for daily log rotation.
// It logs to stdout and to `logs/app-YYYY-MM-DD.log` with a 7-day retention policy.
func NewLogger() *logrus.Logger {
	if err := os.MkdirAll("logs", 0755); err != nil {
		logrus.Errorf("failed to create log directory: %v", err)
	}

	rotator, err := rotatelogs.New(
		"logs/app-%Y-%m-%d.log",
		rotatelogs.WithMaxAge(7*24*time.Hour),     // 7 days retention
		rotatelogs.WithRotationTime(24*time.Hour), // rotate daily
	)
	if err != nil {
		logrus.Errorf("failed to initialize rotate logger: %v", err)
	}

	log := logrus.New()

	if rotator != nil {
		mw := io.MultiWriter(os.Stdout, rotator)
		log.SetOutput(mw)
	} else {
		log.SetOutput(os.Stdout)
	}

	env := os.Getenv("APP_ENV")
	if env == "production" {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
		log.SetLevel(logrus.InfoLevel)
	} else {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			ForceColors:     true,
		})
		log.SetLevel(logrus.DebugLevel)
	}

	return log
}
