package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/service"
)

func TestCalculateNextRunTime(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.UTC
	}

	t.Run("Before target time on the same day", func(t *testing.T) {
		// Mock time: 2026-08-18 03:30:00 (Before 05:00)
		mockNow := time.Date(2026, 8, 18, 3, 30, 0, 0, loc)
		nextRun := service.CalculateNextRunTime(mockNow, 5, 0, loc)

		expected := time.Date(2026, 8, 18, 5, 0, 0, 0, loc)
		if !nextRun.Equal(expected) {
			t.Errorf("expected next run %v, got %v", expected, nextRun)
		}
	})

	t.Run("After target time on the same day", func(t *testing.T) {
		// Mock time: 2026-08-18 07:15:00 (After 05:00)
		mockNow := time.Date(2026, 8, 18, 7, 15, 0, 0, loc)
		nextRun := service.CalculateNextRunTime(mockNow, 5, 0, loc)

		expected := time.Date(2026, 8, 19, 5, 0, 0, 0, loc) // Harus besoknya
		if !nextRun.Equal(expected) {
			t.Errorf("expected next run %v, got %v", expected, nextRun)
		}
	})

	t.Run("Exactly at target time", func(t *testing.T) {
		// Mock time: 2026-08-18 05:00:00
		mockNow := time.Date(2026, 8, 18, 5, 0, 0, 0, loc)
		nextRun := service.CalculateNextRunTime(mockNow, 5, 0, loc)

		expected := time.Date(2026, 8, 19, 5, 0, 0, 0, loc) // Harus besoknya
		if !nextRun.Equal(expected) {
			t.Errorf("expected next run %v, got %v", expected, nextRun)
		}
	})

	t.Run("Custom target time e.g. 13:00 (Siang)", func(t *testing.T) {
		// Mock time: 2026-08-18 10:00:00
		mockNow := time.Date(2026, 8, 18, 10, 0, 0, 0, loc)
		nextRun := service.CalculateNextRunTime(mockNow, 13, 0, loc)

		expected := time.Date(2026, 8, 18, 13, 0, 0, 0, loc)
		if !nextRun.Equal(expected) {
			t.Errorf("expected next run %v, got %v", expected, nextRun)
		}
	})
}

func TestFormatDailyMorningDigest(t *testing.T) {
	mockDate := time.Date(2026, 8, 18, 5, 0, 0, 0, time.UTC)

	t.Run("Empty deliveries list", func(t *testing.T) {
		deliveries := []model.Delivery{}
		text := service.FormatDailyMorningDigest(mockDate, deliveries)

		if !strings.Contains(text, "Tidak ada jadwal pengantaran") {
			t.Errorf("expected empty message, got: %s", text)
		}
		if !strings.Contains(text, "18 Aug 2026") {
			t.Errorf("expected formatted date in message, got: %s", text)
		}
	})

	t.Run("With active deliveries", func(t *testing.T) {
		deliveries := []model.Delivery{
			{
				ID:              1,
				OrderNumber:     "APT-20260818-R0W5",
				CustomerName:    "Bpk. Budi",
				CustomerPhone:   "08123456789",
				CustomerAddress: "Jln Sulfat No 10",
				SlotName:        "Slot Pagi (09:00 - 12:00)",
				DistanceKm:      10.8,
			},
			{
				ID:              2,
				OrderNumber:     "APT-20260818-K2P1",
				CustomerName:    "Ibu Ani",
				CustomerPhone:   "08122334455",
				CustomerAddress: "Jln Bungur No 5",
				SlotName:        "Slot Siang (13:00 - 16:00)",
				DistanceKm:      5.2,
			},
		}

		text := service.FormatDailyMorningDigest(mockDate, deliveries)

		// Assertions
		if !strings.Contains(text, "Total Paket Siap Antar:* 2 Pengiriman") {
			t.Errorf("expected total packages count in message, got: %s", text)
		}
		if !strings.Contains(text, "APT-20260818-R0W5") || !strings.Contains(text, "Bpk. Budi") {
			t.Errorf("expected delivery item 1 in message, got: %s", text)
		}
		if !strings.Contains(text, "APT-20260818-K2P1") || !strings.Contains(text, "Ibu Ani") {
			t.Errorf("expected delivery item 2 in message, got: %s", text)
		}
		if !strings.Contains(text, "10.8 km") {
			t.Errorf("expected distance in message, got: %s", text)
		}
	})
}
