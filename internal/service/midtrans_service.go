package service

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/config"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

type MidtransService struct {
	cfg          config.MidtransConfig
	orderSvc     *OrderService
	customerRepo *repository.CustomerRepository
	msgSender    model.MessageSender
	httpClient   *http.Client
	logger       *zap.Logger
}

func NewMidtransService(
	cfg config.MidtransConfig,
	orderSvc *OrderService,
	customerRepo *repository.CustomerRepository,
	logger *zap.Logger,
) *MidtransService {
	return &MidtransService{
		cfg:          cfg,
		orderSvc:     orderSvc,
		customerRepo: customerRepo,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		logger: logger,
	}
}

func (s *MidtransService) SetMessageSender(sender model.MessageSender) {
	s.msgSender = sender
}

func (s *MidtransService) GetMerchantID() string {
	return s.cfg.MerchantID
}

func (s *MidtransService) GetClientKey() string {
	return s.cfg.ClientKey
}

func (s *MidtransService) IsProduction() bool {
	return s.cfg.IsProduction
}

func (s *MidtransService) GetSnapJSURL() string {
	return s.cfg.SnapJSURL
}

type SnapTransactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int64  `json:"gross_amount"`
}

type SnapItemDetails struct {
	ID       string `json:"id"`
	Price    int64  `json:"price"`
	Quantity int    `json:"quantity"`
	Name     string `json:"name"`
}

type SnapCustomerDetails struct {
	FirstName string `json:"first_name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type SnapRequestPayload struct {
	TransactionDetails SnapTransactionDetails `json:"transaction_details"`
	ItemDetails        []SnapItemDetails      `json:"item_details,omitempty"`
	CustomerDetails    *SnapCustomerDetails   `json:"customer_details,omitempty"`
}

type SnapResponse struct {
	Token         string   `json:"token"`
	RedirectURL   string   `json:"redirect_url"`
	ErrorMessages []string `json:"error_messages,omitempty"`
}

func (s *MidtransService) CreateSnapTransaction(ctx context.Context, orderID int64) (*SnapResponse, error) {
	if s.cfg.ServerKey == "" {
		return nil, fmt.Errorf("MIDTRANS_SERVER_KEY belum dikonfigurasi")
	}

	order, err := s.orderSvc.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order tidak ditemukan: %w", err)
	}

	if order.Status == model.OrderStatusPaid {
		return nil, fmt.Errorf("pesanan ini sudah lunas, tidak dapat dibayar ulang")
	}
	if order.Status == model.OrderStatusCancelled {
		return nil, fmt.Errorf("pesanan ini telah dibatalkan, pembayaran tidak dapat diproses")
	}

	var customer *model.Customer
	if order.UserID.Valid && order.UserID.Int64 > 0 {
		c, err := s.customerRepo.GetByUserID(ctx, order.UserID.Int64)
		if err == nil {
			customer = c
		}
	}

	items := make([]SnapItemDetails, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, SnapItemDetails{
			ID:       fmt.Sprintf("%d", item.ProductID),
			Price:    int64(item.UnitPrice),
			Quantity: item.Quantity,
			Name:     item.ProductName,
		})
	}

	payload := SnapRequestPayload{
		TransactionDetails: SnapTransactionDetails{
			OrderID:     order.OrderNumber,
			GrossAmount: int64(order.TotalPrice),
		},
		ItemDetails: items,
	}

	if customer != nil {
		payload.CustomerDetails = &SnapCustomerDetails{
			FirstName: customer.Name,
			Email:     customer.Email,
			Phone:     customer.Phone,
		}
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gagal encode snap payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.SnapURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request: %w", err)
	}

	authStr := base64.StdEncoding.EncodeToString([]byte(s.cfg.ServerKey + ":"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+authStr)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal mengirim request ke Midtrans: %w", err)
	}
	defer resp.Body.Close()

	var snapResp SnapResponse
	if err := json.NewDecoder(resp.Body).Decode(&snapResp); err != nil {
		return nil, fmt.Errorf("gagal decode response Midtrans: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		s.logger.Error("Midtrans Snap error response",
			zap.Int("status_code", resp.StatusCode),
			zap.Strings("errors", snapResp.ErrorMessages),
		)
		return nil, fmt.Errorf("midtrans error (%d): %v", resp.StatusCode, snapResp.ErrorMessages)
	}

	return &snapResp, nil
}

type MidtransNotification struct {
	TransactionTime   string `json:"transaction_time"`
	TransactionStatus string `json:"transaction_status"`
	StatusMessage     string `json:"status_message"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
	PaymentType       string `json:"payment_type"`
	OrderID           string `json:"order_id"`
	GrossAmount       string `json:"gross_amount"`
	FraudStatus       string `json:"fraud_status,omitempty"`
}

func (s *MidtransService) VerifySignature(notif MidtransNotification) bool {
	if s.cfg.ServerKey == "" {
		s.logger.Error("MIDTRANS_SERVER_KEY kosong, tidak bisa verifikasi signature")
		return false
	}
	input := notif.OrderID + notif.StatusCode + notif.GrossAmount + s.cfg.ServerKey
	hash := sha512.Sum512([]byte(input))
	expectedSignature := hex.EncodeToString(hash[:])

	match := expectedSignature == notif.SignatureKey
	if !match {
		s.logger.Warn("signature mismatch",
			zap.String("order_id", notif.OrderID),
			zap.String("expected", expectedSignature[:16]+"..."),
			zap.String("received", func() string {
				if len(notif.SignatureKey) >= 16 {
					return notif.SignatureKey[:16] + "..."
				}
				return notif.SignatureKey
			}()),
		)
	}
	return match
}

func (s *MidtransService) ProcessNotification(ctx context.Context, notif MidtransNotification) error {
	s.logger.Info("[MIDTRANS] notifikasi diterima",
		zap.String("order_id", notif.OrderID),
		zap.String("transaction_status", notif.TransactionStatus),
		zap.String("status_code", notif.StatusCode),
		zap.String("gross_amount", notif.GrossAmount),
	)

	if !s.VerifySignature(notif) {
		s.logger.Warn("[MIDTRANS] signature GAGAL — status tidak akan diupdate",
			zap.String("order_id", notif.OrderID),
		)
		return fmt.Errorf("signature verification failed")
	}
	s.logger.Info("[MIDTRANS] signature OK", zap.String("order_id", notif.OrderID))

	order, err := s.orderSvc.orderRepo.GetByOrderNumber(ctx, notif.OrderID)
	if err != nil {
		s.logger.Error("[MIDTRANS] order tidak ditemukan di DB",
			zap.String("order_id", notif.OrderID),
			zap.Error(err),
		)
		return fmt.Errorf("order not found: %w", err)
	}
	s.logger.Info("[MIDTRANS] order ditemukan",
		zap.String("order_number", order.OrderNumber),
		zap.String("current_status", string(order.Status)),
	)

	status := notif.TransactionStatus
	fraud := notif.FraudStatus

	if status == "capture" && fraud == "accept" || status == "settlement" {
		s.logger.Info("[MIDTRANS] memproses settlement/capture → paid", zap.String("order_id", notif.OrderID))
		updatedOrder, err := s.orderSvc.ConfirmOrder(ctx, order.ID)
		if err != nil {
			s.logger.Error("[MIDTRANS] ConfirmOrder gagal", zap.Error(err))
			return err
		}

		// Send notification message if messageSender is configured
		if s.msgSender != nil {
			platform := model.PlatformFonnte
			target := ""

			if updatedOrder.TelegramChatID.Valid && updatedOrder.TelegramChatID.String != "" {
				target = updatedOrder.TelegramChatID.String
				platform = model.PlatformTelegram
			} else {
				if updatedOrder.Source == "telegram" {
					platform = model.PlatformTelegram
				}

				if updatedOrder.UserID.Valid && updatedOrder.UserID.Int64 > 0 {
					if c, err := s.customerRepo.GetByUserID(ctx, updatedOrder.UserID.Int64); err == nil && c.Phone != "" {
						target = c.Phone
					}
				}
				if target == "" && updatedOrder.Notes != "" {
					target = updatedOrder.Notes
				}
			}

			if target != "" {
				msg := fmt.Sprintf("✅ *Pembayaran Midtrans Berhasil!*\n\nPembayaran untuk pesanan *%s* sebesar *Rp %s* telah diterima. Pesanan Anda sedang diproses oleh tim gudang. Terima kasih!",
					updatedOrder.OrderNumber, formatIDR(updatedOrder.TotalPrice))
				if err := s.msgSender.SendText(ctx, platform, target, msg); err != nil {
					s.logger.Error("[MIDTRANS] Gagal mengirim pesan notifikasi ke customer", zap.Error(err))
				}
			}
		}
		return nil
	} else if status == "cancel" || status == "deny" || status == "expire" {
		s.logger.Info("[MIDTRANS] memproses cancel/deny/expire", zap.String("order_id", notif.OrderID))
		err = s.orderSvc.CancelOrder(ctx, order.ID)
		if err != nil {
			s.logger.Error("[MIDTRANS] CancelOrder gagal", zap.Error(err))
		}
		return err
	} else {
		s.logger.Info("[MIDTRANS] status tidak ditangani, diabaikan",
			zap.String("transaction_status", status),
		)
	}

	return nil
}
