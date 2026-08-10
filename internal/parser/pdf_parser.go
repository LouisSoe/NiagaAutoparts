package parser

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

// InvoiceData hasil ekstrak dari PDF faktur
type InvoiceData struct {
	// Header faktur
	InvoiceNumber string
	InvoiceDate   string
	SellerName    string
	SellerNPWP    string
	BuyerName     string
	BuyerAddress  string

	// Item barang
	Items []InvoiceItem

	// Total
	Subtotal   float64
	TaxBase    float64 // Dasar Pengenaan Pajak
	PPNAmount  float64 // Jumlah PPN
	PPnBMAmount float64

	// Raw teks jika parsing struktural gagal
	RawText string
}

type InvoiceItem struct {
	Code     string
	Name     string
	Quantity float64
	Unit     string
	UnitPrice float64
	Total    float64
}

// ParsePDF membaca PDF dari bytes dan ekstrak data faktur
func ParsePDF(data []byte) (*InvoiceData, error) {
	// pakai reader langsung
	reader := bytes.NewReader(data)
	r, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pdf open: %w", err)
	}

	// ekstrak semua teks dari semua halaman
	var sb strings.Builder
	totalPages := r.NumPage()
	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	rawText := sb.String()
	if rawText == "" {
		return nil, fmt.Errorf("pdf has no extractable text — may be scanned image, use OCR instead")
	}

	result := &InvoiceData{RawText: rawText}
	extractInvoiceFields(rawText, result)
	return result, nil
}

// extractInvoiceFields parsing field dari teks PDF menggunakan regex
func extractInvoiceFields(text string, inv *InvoiceData) {
	lines := strings.Split(text, "\n")

	// ── Nomor faktur pajak ─────────────────────────────────────────────────
	// Format: "Kode dan Nomor Seri Faktur Pajak: 04002600161820712"
	reFakturNo := regexp.MustCompile(`(?i)nomor seri faktur pajak[:\s]+(\d+)`)
	if m := reFakturNo.FindStringSubmatch(text); len(m) > 1 {
		inv.InvoiceNumber = strings.TrimSpace(m[1])
	}

	// Juga cek format referensi supplier: SG00-080526-00004
	reRefNo := regexp.MustCompile(`Referensi[:\s]+([A-Z0-9\-#/]+)`)
	if m := reRefNo.FindStringSubmatch(text); len(m) > 1 {
		if inv.InvoiceNumber == "" {
			inv.InvoiceNumber = strings.TrimSpace(m[1])
		}
	}

	// ── Tanggal ────────────────────────────────────────────────────────────
	// Format: "22 April 2026" atau "22/04/2026"
	reDate := regexp.MustCompile(`(\d{1,2}\s+(?:Januari|Februari|Maret|April|Mei|Juni|Juli|Agustus|September|Oktober|November|Desember)\s+\d{4})`)
	if m := reDate.FindStringSubmatch(text); len(m) > 1 {
		inv.InvoiceDate = strings.TrimSpace(m[1])
	}

	// ── Seller & Buyer — parse dari baris per baris ───────────────────────
	for i, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Nama : ") && inv.SellerName == "" {
			inv.SellerName = strings.TrimPrefix(line, "Nama : ")
		} else if strings.HasPrefix(line, "NPWP : ") && inv.SellerNPWP == "" {
			inv.SellerNPWP = strings.TrimPrefix(line, "NPWP : ")
		}

		// buyer info muncul setelah "Pembeli Barang Kena Pajak"
		if strings.Contains(line, "Pembeli Barang Kena Pajak") {
			// baris berikutnya adalah data buyer
			for j := i + 1; j < len(lines) && j < i+6; j++ {
				bl := strings.TrimSpace(lines[j])
				if strings.HasPrefix(bl, "Nama : ") && inv.BuyerName == "" {
					inv.BuyerName = strings.TrimPrefix(bl, "Nama : ")
				}
				if strings.HasPrefix(bl, "Alamat : ") && inv.BuyerAddress == "" {
					inv.BuyerAddress = strings.TrimPrefix(bl, "Alamat : ")
				}
			}
		}
	}

	// ── Items / Barang ─────────────────────────────────────────────────────
	inv.Items = extractItems(text)

	// ── Totals ─────────────────────────────────────────────────────────────
	reHargaJual := regexp.MustCompile(`Harga Jual[^0-9]*([0-9.,]+)`)
	if m := reHargaJual.FindStringSubmatch(text); len(m) > 1 {
		inv.Subtotal = parseIDRNumber(m[1])
	}

	reDPP := regexp.MustCompile(`Dasar Pengenaan Pajak\s+([0-9.,]+)`)
	if m := reDPP.FindStringSubmatch(text); len(m) > 1 {
		inv.TaxBase = parseIDRNumber(m[1])
	}

	rePPN := regexp.MustCompile(`Jumlah PPN[^0-9]*([0-9.,]+)`)
	if m := rePPN.FindStringSubmatch(text); len(m) > 1 {
		inv.PPNAmount = parseIDRNumber(m[1])
	}

	rePPnBM := regexp.MustCompile(`Jumlah PPnBM[^0-9]*([0-9.,]+)`)
	if m := rePPnBM.FindStringSubmatch(text); len(m) > 1 {
		inv.PPnBMAmount = parseIDRNumber(m[1])
	}
}

// extractItems ekstrak baris barang dari tabel faktur
func extractItems(text string) []InvoiceItem {
	items := []InvoiceItem{}

	// pola: nomor urut diikuti kode barang dan nama
	// contoh: "1 000000 Blotong PG Glenmore Rp 61.261,00 x 100,00 Metrik Ton"
	reItem := regexp.MustCompile(
		`(\d+)\s+(\d{6})\s+(.+?)\s+Rp\s+([\d.,]+)\s+x\s+([\d.,]+)\s+(\w[\w\s]*)`,
	)

	matches := reItem.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) < 7 {
			continue
		}
		item := InvoiceItem{
			Code:      strings.TrimSpace(m[2]),
			Name:      strings.TrimSpace(m[3]),
			UnitPrice: parseIDRNumber(m[4]),
			Quantity:  parseIDRNumber(m[5]),
			Unit:      strings.TrimSpace(m[6]),
		}
		item.Total = item.UnitPrice * item.Quantity
		items = append(items, item)
	}

	return items
}

// parseIDRNumber konversi "61.261,00" → 61261.0
func parseIDRNumber(s string) float64 {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// FormatInvoiceSummary membuat ringkasan teks untuk dikirim via WhatsApp
func FormatInvoiceSummary(inv *InvoiceData) string {
	var sb strings.Builder

	sb.WriteString("📄 *Faktur Pajak Terdeteksi*\n\n")

	if inv.InvoiceNumber != "" {
		sb.WriteString(fmt.Sprintf("🔢 No. Faktur: `%s`\n", inv.InvoiceNumber))
	}
	if inv.InvoiceDate != "" {
		sb.WriteString(fmt.Sprintf("📅 Tanggal: %s\n", inv.InvoiceDate))
	}
	if inv.SellerName != "" {
		sb.WriteString(fmt.Sprintf("🏭 Penjual: %s\n", inv.SellerName))
	}
	if inv.BuyerName != "" {
		sb.WriteString(fmt.Sprintf("👤 Pembeli: %s\n", inv.BuyerName))
	}

	if len(inv.Items) > 0 {
		sb.WriteString("\n*Barang:*\n")
		for i, item := range inv.Items {
			sb.WriteString(fmt.Sprintf(
				"%d. %s\n   %.2f %s × Rp %s = Rp %s\n",
				i+1,
				item.Name,
				item.Quantity,
				item.Unit,
				formatNumber(item.UnitPrice),
				formatNumber(item.Total),
			))
		}
	}

	sb.WriteString("\n*Ringkasan:*\n")
	if inv.Subtotal > 0 {
		sb.WriteString(fmt.Sprintf("💰 Harga Jual: Rp %s\n", formatNumber(inv.Subtotal)))
	}
	if inv.TaxBase > 0 {
		sb.WriteString(fmt.Sprintf("📊 DPP: Rp %s\n", formatNumber(inv.TaxBase)))
	}
	if inv.PPNAmount > 0 {
		sb.WriteString(fmt.Sprintf("🧾 PPN: Rp %s\n", formatNumber(inv.PPNAmount)))
	}

	total := inv.Subtotal + inv.PPNAmount
	if total > 0 {
		sb.WriteString(fmt.Sprintf("\n✅ *Total: Rp %s*\n", formatNumber(total)))
	}

	return sb.String()
}

func formatNumber(f float64) string {
	// format: 6.126.100,00
	s := fmt.Sprintf("%.2f", f)
	parts := strings.Split(s, ".")
	intPart := parts[0]
	decPart := parts[1]

	// tambah titik pemisah ribuan
	var result []byte
	for i, c := range []byte(intPart) {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, c)
	}
	return string(result) + "," + decPart
}