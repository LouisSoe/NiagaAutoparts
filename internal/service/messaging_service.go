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

	"go.uber.org/zap"
)

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
func (s *MessagingService) SendText(ctx context.Context, to, message string) error {
	payload := map[string]interface{}{
		"target":  to,
		"message": message,
	}
	return s.send(ctx, payload)
}

// SendTyping sends a fast "sedang diproses..." acknowledgement before the full reply.
// This satisfies the <1 second first response requirement.
func (s *MessagingService) SendTyping(ctx context.Context, to string) error {
	return s.SendText(ctx, to, "⏳ _Sedang diproses..._")
}

// SendMedia sends a message that includes a media file (image, audio, video, document).
// This mirrors the PHP sendFonnte() that passes 'url' and 'filename' alongside 'message'.
//
// Example usage:
//
//	svc.SendMedia(ctx, "6281234567890", "Ini foto produk:", "https://example.com/img.jpg", "foto_produk", "")
//
// Parameters:
//   - to       : target WhatsApp number
//   - message  : caption text (can be empty "")
//   - mediaURL : publicly accessible URL of the file
//   - filename : display name of the file (without extension, Fonnte adds it)
//   - mimeType : optional hint, e.g. "image/jpeg" (leave "" to let Fonnte auto-detect)
func (s *MessagingService) SendMedia(ctx context.Context, to, message, mediaURL, filename, mimeType string) error {
	payload := map[string]interface{}{
		"target":   to,
		"message":  message,
		"url":      mediaURL,
		"filename": filename,
	}
	if mimeType != "" {
		payload["type"] = mimeType
	}
	return s.send(ctx, payload)
}

// send is the low-level method that posts to the Fonnte API.
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

	s.logger.Debug("message sent",
		zap.String("to", maskPhone(payload["target"].(string))),
		zap.Int("status", resp.StatusCode),
	)
	return nil
}

// pingClient is a dedicated http.Client for Ping only.
// It intentionally uses a longer timeout (30s) than the hot-path client (5s)
// because connectivity checks can be slow on first dial.
var pingClient = &http.Client{Timeout: 30 * time.Second}

// Ping sends a test message to the given target number and returns the raw
// Fonnte API response. It is intended only for connectivity checks (e.g. the
// GET /fonnte/test endpoint) and should never be called in hot paths.
//
// Unlike send(), Ping uses pingClient (30s timeout) so the caller gets a
// meaningful response even when the first TCP dial to Fonnte is slow.
func (s *MessagingService) Ping(ctx context.Context, target, message string) (*FonnteResponse, error) {
	payload := map[string]interface{}{
		"target":  target,
		"message": message,
	}
 	s.logger.Info("token yang dipakai",
        zap.String("token", s.token), // hapus setelah fix!
    )
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
		// Classify the error so the handler can give actionable hints.
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf(
				"tidak bisa menjangkau api.fonnte.com (timeout setelah 30 detik). "+
					"Pastikan: (1) server punya akses internet, "+
					"(2) firewall/antivirus tidak memblokir port 443, "+
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
		// Return raw body even if unparseable
		return &FonnteResponse{Detail: string(respBytes)}, nil
	}
	return &fonnteResp, nil
}

// maskPhone returns a privacy-safe phone string for logging (e.g. 6281234****90).
func maskPhone(phone string) string {
	if len(phone) < 6 {
		return "****"
	}
	return phone[:6] + "****" + phone[len(phone)-2:]
}