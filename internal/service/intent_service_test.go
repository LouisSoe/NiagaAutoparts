package service

import (
	"context"
	"testing"

	"github.com/louissoe/niaga-autoparts/internal/model"
)

func TestParseMessage(t *testing.T) {
	svc := NewIntentService()

	tests := []struct {
		name           string
		input          string
		expectedIntent model.Intent
	}{
		{
			name:           "Order intent with quantity",
			input:          "pesan 2 kampas rem",
			expectedIntent: model.IntentOrder,
		},
		{
			name:           "Confirm order intent",
			input:          "ya",
			expectedIntent: model.IntentConfirmOrder,
		},
		{
			name:           "Cancel order intent",
			input:          "batal",
			expectedIntent: model.IntentCancelOrder,
		},
		{
			name:           "Check order intent",
			input:          "cek pesanan",
			expectedIntent: model.IntentCheckOrder,
		},
		{
			name:           "Search stock intent",
			input:          "stok oli AHM",
			expectedIntent: model.IntentSearchStock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := svc.ParseMessage(context.Background(), tt.input)
			if parsed.Intent != tt.expectedIntent {
				t.Errorf("ParseMessage(%q) intent = %v; want %v", tt.input, parsed.Intent, tt.expectedIntent)
			}
		})
	}
}
