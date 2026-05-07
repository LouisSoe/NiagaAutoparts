package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"go.uber.org/zap"
)

// Compile-time check: MessagingService must implement model.MessageSender.
var _ model.MessageSender = (*MessagingService)(nil)

// FonnteResponse is the raw JSON response from the Fonnte API.
type FonnteResponse struct {
	Status  bool   `json:"status"`
	Detail  string `json:"detail"`
	ID      string `json:"id"`
	Target  string `json:"target"`
	Process string `json:"process"`
}

// MessagingService handles outbound messages via the Fonnte WhatsApp gateway.
type MessagingService struct {
	token      string
	apiURL     string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewMessagingService constructs a MessagingService.
func NewMessagingService(token, apiURL string, logger *zap.Logger) *MessagingService {
	return &MessagingService{
		token:  token,
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// SendText sends a plain text message to a WhatsApp number.
// The platform argument is accepted to satisfy model.MessageSender but ignored
// since this service only delivers to Fonnte/WhatsApp.
func (s *MessagingService) SendText(ctx context.Context, _ model.Platform, to, message string) error {
	payload := map[string]interface{}{
		"target":  to,
		"message": message,
	}
	return s.send(ctx, payload)
}

// SendMedia sends a message that includes a media file (image, audio, video, document).
// The platform argument is accepted to satisfy model.MessageSender but ignored
// since this service only delivers to Fonnte/WhatsApp.
//
// Example:
//
//	svc.SendMedia(ctx, model.PlatformFonnte, "6281234567890", "Foto produk:", "https://…/img.jpg", "foto_produk", "")
//
// Parameters:
//   - to       : target WhatsApp number
//   - caption  : text shown alongside the file (can be "")
//   - mediaURL : publicly accessible URL of the file
//   - filename : display name of the file (Fonnte adds extension automatically)
//   - mimeType : optional hint, e.g. "image/jpeg" (leave "" to let Fonnte auto-detect)
func (s *MessagingService) SendMedia(ctx context.Context, _ model.Platform, to, caption, mediaURL, filename, mimeType string) error {
	payload := map[string]interface{}{
		"target":   to,
		"message":  caption,
		"url":      mediaURL,
		"filename": filename,
	}
	if mimeType != "" {
		payload["type"] = mimeType
	}
	return s.send(ctx, payload)
}

// ─── Internal ─────────────────────────────────────────────────────────────────

func (s *MessagingService) send(ctx context.Context, payload map[string]interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fonnte http post: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		s.logger.Warn("fonnte api returned non-200",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBytes)),
		)
		return fmt.Errorf("fonnte api error: %d %s", resp.StatusCode, string(respBytes))
	}

	s.logger.Debug("fonnte message sent",
		zap.String("to", maskPhoneFonnte(payload["target"].(string))),
		zap.Int("status", resp.StatusCode),
	)
	return nil
}

// pingClient is a dedicated http.Client for Ping only — higher timeout (30 s)
// than the hot-path client (5 s) to allow slow first-dial connectivity checks.
var pingClient = &http.Client{Timeout: 30 * time.Second}

// Ping sends a test message and returns the raw Fonnte API response.
// Intended only for the GET /fonnte/test diagnostic endpoint.
func (s *MessagingService) Ping(ctx context.Context, target, message string) (*FonnteResponse, error) {
	payload := map[string]interface{}{
		"target":  target,
		"message": message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal ping payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build ping request: %w", err)
	}
	req.Header.Set("Authorization", s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := pingClient.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf(
				"tidak bisa menjangkau api.fonnte.com (timeout setelah 30 detik). "+
					"Pastikan: (1) server punya akses internet, "+
					"(2) firewall tidak memblokir port 443, "+
					"(3) FONNTE_API_URL di .env benar: %w", err)
		}
		return nil, fmt.Errorf("fonnte ping http post: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	s.logger.Info("fonnte ping response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(respBytes)),
	)

	var fonnteResp FonnteResponse
	if err := json.Unmarshal(respBytes, &fonnteResp); err != nil {
		return &FonnteResponse{Detail: string(respBytes)}, nil
	}
	return &fonnteResp, nil
}

// maskPhoneFonnte returns a privacy-safe phone string for logging.
func maskPhoneFonnte(phone string) string {
	if len(phone) < 6 {
		return "****"
	}
	return phone[:6] + "****" + phone[len(phone)-2:]
}