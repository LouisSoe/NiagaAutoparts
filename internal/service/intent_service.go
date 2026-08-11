package service

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/louissoe/niaga-autoparts/internal/ai"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"go.uber.org/zap"
)

// IntentService detects user intent using rule-based matching first,
// falling back to AI only when no rule matches.
type IntentService struct {
	aiSvc  *ai.AIService
	logger *zap.Logger
}

func NewIntentService(aiSvc *ai.AIService, logger *zap.Logger) *IntentService {
	return &IntentService{aiSvc: aiSvc, logger: logger}
}

// ─── Keyword Maps ─────────────────────────────────────────────────────────────

// greetingKeywords triggers a greeting response.
var greetingKeywords = []string{
	"halo", "hai", "hi", "hello", "selamat pagi", "selamat siang",
	"selamat sore", "selamat malam", "assalamualaikum", "permisi",
}

// searchKeywords indicates the user wants to find a product.
var searchKeywords = []string{
	"cari", "ada", "stock", "stok", "tersedia", "search",
	"punya", "jual", "produk", "spare", "part", "sparepart",
}

// priceKeywords indicates the user is asking about price.
var priceKeywords = []string{
	"harga", "price", "berapa", "cost", "brp", "hrg",
}

// orderKeywords indicates the user wants to place an order.
var orderKeywords = []string{
	"beli", "order", "pesan", "purchase", "mau", "minta",
	"butuh", "need", "want",
}

// confirmKeywords indicates confirmation of an order.
var confirmKeywords = []string{
	"ya", "iya", "yes", "ok", "okay", "oke", "setuju", "jadi",
	"lanjut", "konfirmasi", "confirm", "1", "2", "midtrans", "cash", "tunai",
}

// cancelKeywords indicates order cancellation.
var cancelKeywords = []string{
	"batal", "cancel", "tidak", "nggak", "gak", "no", "stop",
	"hapus", "delete",
}

// checkOrderKeywords indicates the user wants to check an existing order.
var checkOrderKeywords = []string{
	"status order", "cek order", "order saya", "pesanan saya",
	"check order", "my order", "cek pesanan", "pesanan tele",
	"/cekorder", "/pesanan", "/myorders", "/orders",
}

// helpKeywords triggers a help menu.
var helpKeywords = []string{
	"help", "bantuan", "menu", "info", "cara", "bagaimana",
}

// importKeywords indicates the user wants to confirm an import.
var importKeywords = []string{
	"import konfirmasi", "konfirmasi import", "import", "ya import",
}

// historyKeywords indicates the user wants to view or export order history.
var historyKeywords = []string{
	"history", "riwayat", "laporan saya", "riwayat pesanan", "rekap pesanan",
}

// ─── Detect ───────────────────────────────────────────────────────────────────

// Detect returns the parsed intent for a given message.
// Rule-based detection runs first; AI is called as a fallback.
func (s *IntentService) Detect(ctx context.Context, msg string, sess *model.Session) (*model.ParsedMessage, error) {
	normalized := normalizeText(msg)

	// State-aware shortcuts: if session is mid-flow, interpret ambiguous input
	// as the expected next step in that flow.
	if result := s.detectByState(normalized, sess); result != nil {
		return result, nil
	}

	// Rule-based intent matching
	if result := s.detectByKeyword(normalized, msg); result != nil {
		return result, nil
	}

	// AI fallback (only when rules don't match)
	s.logger.Debug("falling back to AI intent detection", zap.String("msg", msg))
	result, err := s.aiSvc.DetectIntent(ctx, msg)
	if err != nil {
		s.logger.Warn("AI intent detection failed", zap.Error(err))
		// Graceful fallback: treat as a product search
		return &model.ParsedMessage{
			OriginalText: msg,
			Intent:       model.IntentSearchProduct,
			ProductQuery: msg,
			Confidence:   0.3,
		}, nil
	}
	return result, nil
}

// detectByState resolves intent from the current session state.
// E.g. if the session is awaiting a quantity, any number is treated as a quantity input.
func (s *IntentService) detectByState(normalized string, sess *model.Session) *model.ParsedMessage {
	switch sess.State {
	case model.StateAwaitingQty:
		qty := extractNumber(normalized)
		if qty > 0 {
			return &model.ParsedMessage{
				Intent:     model.IntentOrder,
				Quantity:   qty,
				Confidence: 0.95,
			}
		}

	case model.StateAwaitingConfirm:
		if containsAny(normalized, confirmKeywords) {
			return &model.ParsedMessage{
				OriginalText: normalized,
				Intent:       model.IntentConfirmOrder,
				Confidence:   0.95,
			}
		}
		if containsAny(normalized, cancelKeywords) {
			return &model.ParsedMessage{
				OriginalText: normalized,
				Intent:       model.IntentCancelOrder,
				Confidence:   0.95,
			}
		}

	case model.StateAwaitingProductSelection:
		// Any message that starts with (or contains) an order keyword followed by a
		// number is treated as "select product #N" — we ignore any trailing number
		// so that "PESAN 1 3" is read as "select #1" (not "search for '1 3'").
		// The user can type a quantity AFTER seeing the product detail.
		n := extractFirstNumber(normalized)
		if n > 0 {
			return &model.ParsedMessage{
				Intent:     model.IntentSelectProduct,
				Quantity:   n, // used as product selection index
				Confidence: 0.99,
			}
		}
	}
	return nil
}

// detectByKeyword maps message keywords to intents.
func (s *IntentService) detectByKeyword(normalized, original string) *model.ParsedMessage {
	switch {
	case containsAny(normalized, greetingKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentGreeting,
			Confidence:   0.99,
		}

	case containsAny(normalized, helpKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentHelp,
			Confidence:   0.99,
		}

	case containsAny(normalized, importKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentConfirmImport,
			Confidence:   0.95,
		}

	case containsAny(normalized, historyKeywords):
		m, y := parseMonthYear(normalized)
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentHistory,
			Month:        m,
			Year:         y,
			Confidence:   0.95,
		}

	case containsAny(normalized, checkOrderKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentCheckOrder,
			Confidence:   0.9,
		}

	case containsAny(normalized, cancelKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentCancelOrder,
			Confidence:   0.9,
		}

	case containsAny(normalized, orderKeywords):
		qty := extractNumber(normalized)
		productQuery := extractProductQuery(normalized, orderKeywords)
		// If everything left after stripping the order keyword is a bare number,
		// the user is specifying a quantity for the last-viewed product, not a name.
		// e.g. "PESAN 2" → qty=2, productQuery="" → handleOrder uses LastProductID.
		if isNumericOnly(strings.TrimSpace(productQuery)) {
			productQuery = ""
		}
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentOrder,
			ProductQuery: productQuery,
			Quantity:     qty,
			Confidence:   0.88,
		}

	case containsAny(normalized, confirmKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentConfirmOrder,
			Confidence:   0.85,
		}

	case containsAny(normalized, priceKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentAskPrice,
			ProductQuery: extractProductQuery(normalized, priceKeywords),
			Confidence:   0.88,
		}

	case containsAny(normalized, searchKeywords):
		return &model.ParsedMessage{
			OriginalText: original,
			Intent:       model.IntentSearchProduct,
			ProductQuery: extractProductQuery(normalized, searchKeywords),
			Confidence:   0.85,
		}
	}
	return nil
}

// ─── Text Utilities ───────────────────────────────────────────────────────────

// normalizeText lowercases and trims the message for keyword matching.
func normalizeText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// containsAny returns true if text contains any of the given keywords.
func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// extractNumber finds the first integer in a string (for quantities).
func extractNumber(s string) int {
	var numStr strings.Builder
	for _, ch := range s {
		if unicode.IsDigit(ch) {
			numStr.WriteRune(ch)
		} else if numStr.Len() > 0 {
			break
		}
	}
	if numStr.Len() == 0 {
		return 0
	}
	var n int
	fmt.Sscanf(numStr.String(), "%d", &n)
	return n
}

// extractFirstNumber returns the value of the first whitespace-delimited numeric
// token in s (e.g. "pesan 1 3" → 1). Unlike extractNumber it does not merge
// adjacent digit sequences, so it cannot accidentally combine "1" and "3" into 13.
func extractFirstNumber(s string) int {
	for _, field := range strings.Fields(s) {
		var n int
		if _, err := fmt.Sscanf(field, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// extractProductQuery strips trigger keywords and returns the remaining text
// as the product search query.
func extractProductQuery(text string, triggers []string) string {
	for _, kw := range triggers {
		text = strings.ReplaceAll(text, kw, "")
	}
	return strings.TrimSpace(text)
}

// isNumericOnly returns true if s consists solely of digit characters (no letters).
// Used to detect when a stripped order query is just a quantity, not a product name.
func isNumericOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return true
}

var monthNamesMap = map[string]int{
	"januari": 1, "jan": 1, "01": 1, "1": 1,
	"februari": 2, "feb": 2, "02": 2, "2": 2,
	"maret": 3, "mar": 3, "03": 3, "3": 3,
	"april": 4, "apr": 4, "04": 4, "4": 4,
	"mei": 5, "may": 5, "05": 5, "5": 5,
	"juni": 6, "jun": 6, "06": 6, "6": 6,
	"juli": 7, "jul": 7, "07": 7, "7": 7,
	"agustus": 8, "agu": 8, "aug": 8, "08": 8, "8": 8,
	"september": 9, "sep": 9, "09": 9, "9": 9,
	"oktober": 10, "okt": 10, "oct": 10, "10": 10,
	"november": 11, "nov": 11, "11": 11,
	"desember": 12, "des": 12, "dec": 12, "12": 12,
}

func parseMonthYear(s string) (int, int) {
	fields := strings.Fields(s)
	month := 0
	year := 0
	for _, f := range fields {
		fClean := strings.Trim(f, ",.-/")
		if len(fClean) == 4 && (strings.HasPrefix(fClean, "20") || strings.HasPrefix(fClean, "21")) {
			var y int
			if _, err := fmt.Sscanf(fClean, "%d", &y); err == nil {
				year = y
			}
		} else if m, ok := monthNamesMap[strings.ToLower(fClean)]; ok {
			month = m
		}
	}
	return month, year
}