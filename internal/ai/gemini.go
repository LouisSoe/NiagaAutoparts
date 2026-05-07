package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"go.uber.org/zap"
)

// GeminiClient is a thin wrapper over the Google Generative AI REST API.
// We use raw HTTP instead of the official SDK to minimise dependencies and
// keep startup time fast.

// IntentResult is the structured JSON Gemini returns for intent classification.
type IntentResult struct {
	Intent       string  `json:"intent"`
	ProductQuery string  `json:"product_query"`
	Quantity     int     `json:"quantity"`
	Confidence   float64 `json:"confidence"`
}

// ImageMatchResult is Gemini's response when analysing a product image.
// PossibleProductsID holds Indonesian names and PossibleProductsEN holds
// English names so both can be used as search queries against the database.
type ImageMatchResult struct {
	PossibleProductsID []string `json:"possible_products_id"` // Indonesian names
	PossibleProductsEN []string `json:"possible_products_en"` // English names
	Confidence         float64  `json:"confidence"`
	Description        string   `json:"description"`
}

// AIService wraps Google AI Studio (Gemini Flash Lite) calls.
type AIService struct {
	apiKey  string
	model   string
	timeout time.Duration
	logger  *zap.Logger
}

// NewAIService constructs an AIService.
func NewAIService(apiKey, modelName string, timeout time.Duration, logger *zap.Logger) *AIService {
	return &AIService{
		apiKey:  apiKey,
		model:   modelName,
		timeout: timeout,
		logger:  logger,
	}
}

// ─── Intent Detection ─────────────────────────────────────────────────────────

// DetectIntent sends a message to Gemini to classify the user's intent.
// This is called ONLY when rule-based detection returns IntentUnknown.
func (s *AIService) DetectIntent(ctx context.Context, message string) (*model.ParsedMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	prompt := buildIntentPrompt(message)
	rawJSON, err := s.callGemini(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("gemini detect intent: %w", err)
	}

	var result IntentResult
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		// Gemini occasionally wraps JSON in markdown fences — strip and retry
		cleaned := stripJSONFences(rawJSON)
		if err2 := json.Unmarshal([]byte(cleaned), &result); err2 != nil {
			s.logger.Warn("failed to parse intent JSON", zap.String("raw", rawJSON), zap.Error(err2))
			return nil, fmt.Errorf("parse intent response: %w", err2)
		}
	}

	return &model.ParsedMessage{
		OriginalText: message,
		Intent:       model.Intent(result.Intent),
		ProductQuery: result.ProductQuery,
		Quantity:     result.Quantity,
		Confidence:   result.Confidence,
		FromAI:       true,
	}, nil
}

// ─── Image Identification ─────────────────────────────────────────────────────

// IdentifyProductFromImageURL downloads an image URL and asks Gemini to identify
// the automotive spare part in it.
func (s *AIService) IdentifyProductFromImageURL(ctx context.Context, imageURL string) (*ImageMatchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// 1. Download image and encode to base64
	b64Data, mimeType, err := downloadImageAsBase64(ctx, imageURL)
	if err != nil {
		s.logger.Warn("failed to download image for AI", zap.Error(err), zap.String("url", imageURL))
		return nil, fmt.Errorf("download image: %w", err)
	}

	prompt := `You are an automotive spare parts expert.
Look at this image and identify what spare part(s) it shows.
Respond ONLY with a valid JSON object (no markdown, no extra text):
{
  "possible_products_id": ["nama suku cadang dalam bahasa Indonesia 1", "nama 2"],
  "possible_products_en": ["part name in English 1", "name 2"],
  "confidence": 0.85,
  "description": "Brief description of the part visible in the image"
}
Rules:
- possible_products_id: list the part names in Indonesian (e.g. "kampas rem", "filter oli", "busi")
- possible_products_en: list the part names in English (e.g. "brake pad", "oil filter", "spark plug")
- Include brand or vehicle type if clearly visible (e.g. "kampas rem Honda Beat")
- List 1-4 candidates per language, ordered by likelihood`

	// 2. Call Gemini with both Text and Image (inlineData)
	rawJSON, err := s.callGeminiWithImage(ctx, prompt, b64Data, mimeType)
	if err != nil {
		return nil, fmt.Errorf("gemini identify image: %w", err)
	}

	var result ImageMatchResult
	cleaned := stripJSONFences(rawJSON)
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse image response: %w", err)
	}
	return &result, nil
}

// ─── Internal HTTP call ───────────────────────────────────────────────────────

// callGemini sends a prompt to the Gemini REST API and returns the raw text response.
func (s *AIService) callGemini(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.1,
			"maxOutputTokens": 256,
		},
	}
	return s.doGeminiRequest(ctx, reqBody)
}

// callGeminiWithImage sends a prompt + image to Gemini.
func (s *AIService) callGeminiWithImage(ctx context.Context, prompt, b64Data, mimeType string) (string, error) {
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
					{
						"inlineData": map[string]string{
							"mimeType": mimeType,
							"data":     b64Data,
						},
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.1,
			"maxOutputTokens": 256,
		},
	}
	return s.doGeminiRequest(ctx, reqBody)
}

// doGeminiRequest is the common function to post to Gemini.
func (s *AIService) doGeminiRequest(ctx context.Context, reqBody map[string]interface{}) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		s.model, s.apiKey,
	)

	respBody, err := doHTTPPost(ctx, url, bodyBytes)
	if err != nil {
		return "", err
	}

	var envelope struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return "", fmt.Errorf("parse gemini envelope: %w", err)
	}
	if envelope.Error != nil {
		return "", fmt.Errorf("gemini api error: %s", envelope.Error.Message)
	}
	if len(envelope.Candidates) == 0 || len(envelope.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned empty response")
	}

	return envelope.Candidates[0].Content.Parts[0].Text, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func buildIntentPrompt(message string) string {
	return fmt.Sprintf(`
You are an assistant for an automotive spare parts store in Indonesia.
The customer sends messages in Indonesian or English.

Classify the intent and extract entities from this message.
Return ONLY a valid JSON object (no markdown backticks, no extra text):
{
  "intent": "<one of: search_product, ask_price, order, check_order, confirm_order, cancel_order, greeting, help, unknown>",
  "product_query": "<extracted product name/keyword or empty string>",
  "quantity": <integer, 0 if not mentioned>,
  "confidence": <float 0.0-1.0>
}

Customer message: "%s"
`, message)
}

// stripJSONFences removes markdown code fences that Gemini sometimes adds.
func stripJSONFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}