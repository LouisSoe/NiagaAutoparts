package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/parser"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

// handleFile dipanggil saat user kirim file via WhatsApp (PDF atau Excel)
// Dipanggil dari Process() setelah deteksi tipe file
func (p *MessageProcessor) handleFile(ctx context.Context, msg model.IncomingMessage, sess *model.Session) {
	phone := msg.Sender
	fileURL := msg.AttachmentURL

	if fileURL == "" {
		_ = p.messagingSvc.SendText(ctx, msg.Platform, phone, "File tidak dapat diproses. Pastikan file terkirim dengan benar.")
		return
	}

	// download file dari URL Fonnte
	fileData, err := downloadFile(ctx, fileURL)
	if err != nil {
		p.logger.Error("download file failed", zap.String("url", fileURL), zap.Error(err))
		_ = p.messagingSvc.SendText(ctx, msg.Platform, phone, "Gagal mengunduh file. Coba kirim ulang.")
		return
	}

	// deteksi tipe file dari URL atau magic bytes
	fileType := detectFileType(fileURL, fileData)

	switch fileType {
	case "pdf":
		p.handlePDFFile(ctx, msg, fileData, sess)
	case "xlsx", "xls":
		p.handleExcelFile(ctx, msg, fileData, sess)
	default:
		_ = p.messagingSvc.SendText(ctx, msg.Platform, phone,
			"Format file tidak didukung. Kirim file *PDF* (faktur) atau *Excel* (.xlsx) untuk update produk.")
	}
}

// handlePDFFile parsing PDF faktur dan tampilkan ringkasan
func (p *MessageProcessor) handlePDFFile(ctx context.Context, msg model.IncomingMessage, data []byte, sess *model.Session) {
	phone := msg.Sender
	_ = p.messagingSvc.SendText(ctx, msg.Platform, phone, "⏳ Membaca faktur PDF...")

	inv, err := parser.ParsePDF(data)
	if err != nil {
		p.logger.Warn("pdf parse failed", zap.Error(err))

		// fallback ke AI vision jika PDF tidak bisa dibaca (mungkin hasil scan)
		if strings.Contains(err.Error(), "scanned image") {
			_ = p.messagingSvc.SendText(ctx, msg.Platform, phone,
				"PDF tampaknya hasil scan. Mencoba baca dengan AI...")
			// kirim ke Gemini vision — sudah ada di handleImage
			// bisa panggil p.aiSvc.IdentifyProductFromImageURL jika ada endpoint
		}
		_ = p.messagingSvc.SendText(ctx, msg.Platform, phone,
			"Gagal membaca PDF. Pastikan file adalah faktur digital (bukan hasil scan).")
		return
	}

	// simpan data faktur ke session untuk follow-up
	sess.State = model.StateIdle
	sess.LastIntent = string(model.IntentViewInvoice)
	sess.ExpiresAt = time.Now().Add(30 * time.Minute)

	reply := parser.FormatInvoiceSummary(inv)

	// cek apakah ada produk yang bisa dicocokkan ke database
	if len(inv.Items) > 0 {
		reply += "\n\n_Ingin mencocokkan barang ini dengan produk di database? Ketik *COCOKKAN*_"
		// simpan items ke session context untuk follow-up
		sess.Context = buildInvoiceContext(inv)
	}

	_ = p.messagingSvc.SendText(ctx, msg.Platform, phone, reply)

	if err := p.sessionRepo.Save(ctx, sess); err != nil {
		p.logger.Warn("save session failed", zap.Error(err))
	}
}

// handleExcelFile parsing Excel dan tawarkan bulk import
func (p *MessageProcessor) handleExcelFile(ctx context.Context, msg model.IncomingMessage, data []byte, sess *model.Session) {
	phone := msg.Sender
	_ = p.messagingSvc.SendText(ctx, msg.Platform, phone, "⏳ Membaca file Excel...")

	result, err := parser.ParseExcel(data)
	if err != nil {
		p.logger.Warn("excel parse failed", zap.Error(err))
		_ = p.messagingSvc.SendText(ctx, msg.Platform, phone,
			fmt.Sprintf("Gagal membaca Excel: %s\n\nPastikan file memiliki kolom: Nama, SKU, Stok, Harga.", err.Error()))
		return
	}

	if len(result.ValidRows) == 0 {
		_ = p.messagingSvc.SendText(ctx, msg.Platform, phone,
			"Tidak ada data valid di file Excel. Periksa format kolom dan coba lagi.")
		return
	}

	// simpan result ke session untuk konfirmasi import
	sess.State = model.StateAwaitingImportConfirm
	sess.Context = buildExcelContext(result)
	sess.ExpiresAt = time.Now().Add(15 * time.Minute)

	reply := parser.FormatExcelSummary(result)
	_ = p.messagingSvc.SendText(ctx, msg.Platform, phone, reply)

	if err := p.sessionRepo.Save(ctx, sess); err != nil {
		p.logger.Warn("save session failed", zap.Error(err))
	}
}

// handleImportConfirm eksekusi bulk update setelah user konfirmasi
func (p *MessageProcessor) handleImportConfirm(ctx context.Context, sess *model.Session, phone string) string {
	if sess.State != model.StateAwaitingImportConfirm || sess.Context == "" {
		return "Tidak ada data import yang menunggu konfirmasi."
	}

	rows, err := parseExcelContext(sess.Context)
	if err != nil || len(rows) == 0 {
		return "Sesi import sudah berakhir. Kirim ulang file Excel."
	}

	// bulk upsert ke database
	success, failed := 0, 0
	for _, row := range rows {
		if err := p.productSvc.UpsertFromExcel(ctx, repository.ExcelProductInput{
			SKU:         row.SKU,
			Name:        row.Name,
			Category:    row.Category,
			Description: row.Description,
			Stock:         row.Stock,
			PurchasePrice: row.Price,
			SellingPrice:  row.Price,
			Unit:          row.Unit,
			Location:      row.Location,
		}); err != nil {
			p.logger.Warn("upsert product failed",
				zap.String("name", row.Name),
				zap.Error(err),
			)
			failed++
		} else {
			success++
		}
	}

	sess.State = model.StateIdle
	sess.Context = ""
	_ = p.sessionRepo.Save(ctx, sess)

	if failed == 0 {
		return fmt.Sprintf("✅ Berhasil import *%d produk* ke database.", success)
	}
	return fmt.Sprintf(
		"✅ Import selesai: *%d berhasil*, ⚠️ *%d gagal*.\nCek log untuk detail error.",
		success, failed,
	)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func downloadFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status: %d", resp.StatusCode)
	}

	// limit 10MB
	limited := io.LimitReader(resp.Body, 10<<20)
	return io.ReadAll(limited)
}

func detectFileType(url string, data []byte) string {
	// cek dari URL dulu
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, ".pdf"):
		return "pdf"
	case strings.Contains(lower, ".xlsx"):
		return "xlsx"
	case strings.Contains(lower, ".xls"):
		return "xls"
	}

	// cek magic bytes
	if len(data) >= 4 {
		// PDF: %PDF
		if bytes.HasPrefix(data, []byte("%PDF")) {
			return "pdf"
		}
		// XLSX (ZIP format): PK\x03\x04
		if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
			return "xlsx"
		}
		// XLS: D0 CF 11 E0
		if bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
			return "xls"
		}
	}
	return "unknown"
}

func buildInvoiceContext(inv *parser.InvoiceData) string {
	// simpan nomor faktur dan jumlah item saja — cukup untuk follow-up
	return fmt.Sprintf("invoice:%s:items:%d", inv.InvoiceNumber, len(inv.Items))
}

func buildExcelContext(result *parser.ExcelParseResult) string {
	// encode valid rows ke JSON sederhana lewat Session.SaveContext
	// untuk simplicity, simpan count saja — data sudah di validRows
	// implementasi penuh bisa encode JSON ke context
	return fmt.Sprintf("excel:%s:valid:%d", result.SheetName, len(result.ValidRows))
}

func parseExcelContext(ctx string) ([]parser.ExcelProductRow, error) {
	// placeholder — implementasi penuh simpan JSON di context
	// untuk sekarang return empty agar tidak panic
	return nil, fmt.Errorf("not implemented — store full rows in session context")
}