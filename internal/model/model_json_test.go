package model

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestCategoryMarshalJSON(t *testing.T) {
	t.Run("Valid Description", func(t *testing.T) {
		cat := Category{
			ID:          1,
			Name:        "Rem",
			Slug:        "rem",
			Description: sql.NullString{String: "Sistem pengereman", Valid: true},
		}

		bytes, err := json.Marshal(cat)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(bytes, &parsed); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if parsed["name"] != "Rem" || parsed["slug"] != "rem" || parsed["description"] != "Sistem pengereman" {
			t.Errorf("Unexpected unmarshaled JSON: %v", parsed)
		}
	})

	t.Run("Invalid Description (Null)", func(t *testing.T) {
		cat := Category{
			ID:          1,
			Name:        "Rem",
			Slug:        "rem",
			Description: sql.NullString{Valid: false},
		}

		bytes, err := json.Marshal(cat)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(bytes, &parsed); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if parsed["name"] != "Rem" || parsed["slug"] != "rem" || parsed["description"] != nil {
			t.Errorf("Unexpected unmarshaled JSON: %v", parsed)
		}
	})
}

func TestOrderMarshalJSON(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	order := Order{
		ID:            1,
		OrderNumber:   "APT-12345",
		UserID:        sql.NullInt64{Int64: 10, Valid: true},
		TotalPrice:    50000,
		AmountPaid:    50000,
		ChangeAmount:  0,
		Status:        OrderStatusPaid,
		Source:        "pos",
		PaymentMethod: sql.NullString{String: "qris", Valid: true},
		CreatedAt:     now,
		UpdatedAt:     now,
		Items: []OrderDetail{
			{
				ID:          1,
				OrderID:     1,
				ProductID:   5,
				ProductName: "Busi NGK",
				Quantity:    2,
				UnitPrice:   25000,
				Subtotal:    50000,
				CreatedAt:   now,
			},
		},
	}

	bytes, err := json.Marshal(order)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("Unmarshal parsed error: %v", err)
	}

	if parsed["user_id"] != float64(10) {
		t.Errorf("expected user_id 10, got %v", parsed["user_id"])
	}
	if parsed["payment_method"] != "qris" {
		t.Errorf("expected payment_method 'qris', got %v", parsed["payment_method"])
	}

	items, ok := parsed["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item in JSON, got %v", parsed["items"])
	}
}

func TestDashboardDataMarshalJSON(t *testing.T) {
	data := DashboardData{
		Summary: DashboardSummary{
			TotalProducts:      50,
			TotalCategories:    5,
			TotalCustomers:     10,
			TotalOrders:        20,
			PendingOrders:      2,
			PaidOrders:         15,
			CancelledOrders:    3,
			TotalRevenue:       1500000,
			LowStockProducts:   4,
			OutOfStockProducts: 1,
		},
		RecentOrders: []RecentOrderSummary{
			{
				ID:           1,
				OrderNumber:  "ORD-001",
				CustomerName: "John Doe",
				TotalPrice:   100000,
				Status:       "paid",
			},
		},
		LowStockItems: []LowStockItem{
			{
				ID:           10,
				SKU:          "SKU-010",
				Name:         "Oli Mesin",
				Category:     "Pelumas",
				Stock:        2,
				Reserved:     0,
				Available:    2,
				MinimumStock: 5,
				SellingPrice: 50000,
			},
		},
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	summary, ok := parsed["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected summary object in JSON")
	}

	if summary["total_products"] != float64(50) {
		t.Errorf("expected total_products 50, got %v", summary["total_products"])
	}
	if summary["total_revenue"] != float64(1500000) {
		t.Errorf("expected total_revenue 1500000, got %v", summary["total_revenue"])
	}
}

