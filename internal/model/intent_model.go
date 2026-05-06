package model

// Intent represents detected user intention.
type Intent string
 
const (
	IntentSearchProduct Intent = "search_product"
	IntentAskPrice      Intent = "ask_price"
	IntentOrder         Intent = "order"
	IntentCheckOrder    Intent = "check_order"
	IntentConfirmOrder  Intent = "confirm_order"
	IntentCancelOrder   Intent = "cancel_order"
	IntentIdentifyImage Intent = "identify_image"
	IntentGreeting      Intent = "greeting"
	IntentHelp          Intent = "help"
	IntentUnknown       Intent = "unknown"
	IntentSelectProduct Intent = "select_product"
)
 
// ParsedMessage is the result of intent detection.
type ParsedMessage struct {
	OriginalText string
	Intent       Intent
	ProductQuery string  // Extracted product keyword
	Quantity     int     // Extracted quantity (for orders)
	Confidence   float64 // 0.0–1.0
	FromAI       bool    // True if intent was detected by AI (not rule-based)
}