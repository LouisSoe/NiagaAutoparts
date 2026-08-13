package logger

import (
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger creates a new logrus.Logger configured with Lumberjack for log rotation.
// It logs to stdout as text and to `logs/app.log` as JSON with a 7-day retention policy.
func NewLogger() *logrus.Logger {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		logrus.Errorf("failed to create log directory: %v", err)
	}

	logFilePath := filepath.Join(logDir, "app.log")

	// Lumberjack handles log rotation and retention
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10,   // Max megabytes before rotation
		MaxBackups: 14,   // Max number of old log files to retain
		MaxAge:     7,    // Max number of days to retain old log files (7 days retention)
		Compress:   true, // Compress old log files with gzip
	}

	log := logrus.New()

	// Write to both Console (stdout) and File
	mw := io.MultiWriter(os.Stdout, lumberjackLogger)
	log.SetOutput(mw)

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
