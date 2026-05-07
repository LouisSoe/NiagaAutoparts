package service

import (
	"fmt"
	"strings"

	"github.com/louissoe/niaga-autoparts/internal/model"
)

// FormatProductList builds a user-friendly WhatsApp message listing multiple products.
func FormatProductList(products []model.Product) string {
	if len(products) == 0 {
		return "Maaf, produk yang Anda cari tidak ditemukan. 😔\n\nCoba gunakan kata kunci lain atau ketik *MENU* untuk melihat kategori."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ Ditemukan *%d produk*:\n\n", len(products)))

	for i, p := range products {
		avail := p.AvailableStock()
		var stockLabel string
		switch {
		case avail == 0:
			stockLabel = "Habis ❌"
		case avail <= 3:
			stockLabel = fmt.Sprintf("Sisa %d %s ⚠️", avail, p.Unit)
		default:
			stockLabel = fmt.Sprintf("Ada Stok ✅ (%d %s tersedia)", avail, p.Unit)
		}

		sb.WriteString(fmt.Sprintf(
			"*%d. %s*\n"+
				"   SKU: %s\n"+
				"   Harga: Rp %s / %s\n"+
				"   Lokasi: %s\n"+
				"   Stok: %s\n\n",
			i+1,
			p.Name,
			p.SKU,
			formatIDR(p.Price),
			p.Unit,
			p.Location,
			stockLabel,
		))
	}

	sb.WriteString("Balas dengan *nomor produk* untuk melihat detail dan memesan.")
	return sb.String()
}


// FormatProductDetail builds a detailed product card message.
func FormatProductDetail(p *model.Product, refs []model.PriceReference) string {
	var sb strings.Builder

	stockStatus := "✅ Tersedia"
	if p.AvailableStock() == 0 {
		stockStatus = "❌ Habis"
	}

	sb.WriteString(fmt.Sprintf(
		"🔧 *%s*\n"+
			"━━━━━━━━━━━━━━━━\n"+
			"SKU      : %s\n"+
			"Kategori : %s\n"+
			"Harga    : *Rp %s / %s*\n"+
			"Stok     : %s (%d %s tersedia)\n"+
			"Lokasi   : %s\n",
		p.Name,
		p.SKU,
		p.Category,
		formatIDR(p.Price), p.Unit,
		stockStatus, p.AvailableStock(), p.Unit,
		p.Location,
	))

	if p.Description.Valid && p.Description.String != "" {
		sb.WriteString(fmt.Sprintf("Info     : %s\n", p.Description.String))
	}

	// Marketplace price comparison
	if len(refs) > 0 {
		sb.WriteString("\n📊 *Perbandingan Harga Marketplace:*\n")
		for _, ref := range refs {
			diff := p.Price - ref.Price
			indicator := "💰 Lebih murah di sini"
			if diff < 0 {
				indicator = fmt.Sprintf("⬆️ +Rp %s di sini", formatIDR(-diff))
			} else if diff == 0 {
				indicator = "➡️ Sama"
			}
			sb.WriteString(fmt.Sprintf("  • %s: Rp %s %s\n",
				capitalise(ref.Marketplace), formatIDR(ref.Price), indicator))
		}
	}

	sb.WriteString("\nKetik *PESAN [jumlah]* untuk memesan produk ini.")
	return sb.String()
}

// FormatOrderConfirmation sends the reservation summary to the user.
func FormatOrderConfirmation(order *model.Order) string {
	return fmt.Sprintf(
		"🛒 *Konfirmasi Pesanan*\n"+
			"━━━━━━━━━━━━━━━━\n"+
			"No. Pesanan : *%s*\n"+
			"Produk      : %s\n"+
			"Jumlah      : %d pcs\n"+
			"Harga Satuan: Rp %s\n"+
			"*Total      : Rp %s*\n"+
			"━━━━━━━━━━━━━━━━\n"+
			"⏰ Reservasi berlaku 15 menit.\n\n"+
			"Balas *YA* untuk konfirmasi atau *BATAL* untuk membatalkan.",
		order.OrderNumber,
		order.ProductName,
		order.Quantity,
		formatIDR(order.UnitPrice),
		formatIDR(order.TotalPrice),
	)
}

// FormatOrderSuccess returns the success message after payment confirmation.
func FormatOrderSuccess(order *model.Order) string {
	return fmt.Sprintf(
		"✅ *Pesanan Dikonfirmasi!*\n\n"+
			"No. Pesanan : *%s*\n"+
			"Produk      : %s\n"+
			"Total       : Rp %s\n\n"+
			"Tim kami akan segera memproses pesanan Anda.\n"+
			"Lokasi pengambilan akan diinformasikan oleh admin.\n\n"+
			"Terima kasih! 🙏",
		order.OrderNumber,
		order.ProductName,
		formatIDR(order.TotalPrice),
	)
}

// FormatWelcome is the greeting message.
func FormatWelcome(name string) string {
	if name == "" {
		name = "Kawan"
	}
	return fmt.Sprintf(
		"Halo *%s*! 👋\n\n"+
			"Selamat datang di *Toko Sparepart Auto* 🔧🚗\n\n"+
			"Silakan ketik:\n"+
			"• *CARI [nama produk]* — Cari produk\n"+
			"• *HARGA [nama produk]* — Cek harga\n"+
			"• *ORDER [nama produk] [jumlah]* — Pesan\n"+
			"• *MENU* — Bantuan\n\n"+
			"Atau kirim *foto* suku cadang untuk identifikasi otomatis! 📷",
		name,
	)
}

// FormatHelp returns the help menu.
func FormatHelp() string {
	return "📋 *Menu Bantuan*\n\n" +
		"*Pencarian Produk:*\n" +
		"  CARI kampas rem\n" +
		"  ADA filter oli honda beat\n\n" +
		"*Cek Harga:*\n" +
		"  HARGA busi NGK\n" +
		"  BERAPA harga aki\n\n" +
		"*Pemesanan:*\n" +
		"  PESAN kampas rem 2\n" +
		"  ORDER filter oli 1\n\n" +
		"*Status Pesanan:*\n" +
		"  CEK ORDER\n" +
		"  STATUS ORDER APT-20240115-A3F9\n\n" +
		"*Identifikasi Foto:*\n" +
		"  Kirim foto suku cadang untuk diidentifikasi 📷\n\n" +
		"Hubungi admin: 08xxx-xxxx-xxxx"
}

// formatBilingualCandidates writes the bilingual AI identification header.
func formatBilingualCandidates(sb *strings.Builder, candidatesID, candidatesEN []string) {
	sb.WriteString("🔍 *Foto diidentifikasi oleh AI:*\n")
	if len(candidatesID) > 0 {
		sb.WriteString("🇮🇩 *Bahasa Indonesia:*\n")
		for _, c := range candidatesID {
			sb.WriteString(fmt.Sprintf("  • %s\n", c))
		}
	}
	if len(candidatesEN) > 0 {
		sb.WriteString("🇬🇧 *English:*\n")
		for _, c := range candidatesEN {
			sb.WriteString(fmt.Sprintf("  • %s\n", c))
		}
	}
}

// FormatImageNoDBMatch is shown when Gemini recognised a part but the DB
// returned no matching products. Shows bilingual suggestions for manual follow-up.
func FormatImageNoDBMatch(candidatesID, candidatesEN []string) string {
	var sb strings.Builder
	formatBilingualCandidates(&sb, candidatesID, candidatesEN)
	sb.WriteString("\n❌ Produk tersebut belum ditemukan di database kami.\n")
	sb.WriteString("Coba ketik *CARI [nama produk]* untuk mencari dengan kata kunci lain.")
	return sb.String()
}

// FormatImageFoundSingle is shown when the DB search returns exactly one product.
// Displays the full product card directly — the user does not need to pick a number.
func FormatImageFoundSingle(candidatesID, candidatesEN []string, p *model.Product, refs []model.PriceReference) string {
	var sb strings.Builder
	formatBilingualCandidates(&sb, candidatesID, candidatesEN)
	sb.WriteString("\n✅ *Ditemukan di database:*\n\n")
	sb.WriteString(FormatProductDetail(p, refs))
	return sb.String()
}

// FormatImageFoundMultiple is shown when the DB search returns several matching
// products. Shows a numbered list so the user can select one by number.
func FormatImageFoundMultiple(candidatesID, candidatesEN []string, products []model.Product) string {
	var sb strings.Builder
	formatBilingualCandidates(&sb, candidatesID, candidatesEN)
	sb.WriteString("\n")
	sb.WriteString(FormatProductList(products))
	return sb.String()
}


// FormatError returns a user-friendly error message.
func FormatError(code string) string {
	msgs := map[string]string{
		"not_found":      "Produk tidak ditemukan. Coba kata kunci yang berbeda.",
		"out_of_stock":   "Maaf, produk sedang habis. Hubungi admin untuk informasi restok.",
		"order_failed":   "Gagal membuat pesanan. Silakan coba lagi atau hubungi admin.",
		"db_error":       "Terjadi gangguan teknis. Silakan coba beberapa saat lagi.",
		"session_error":  "Sesi Anda telah berakhir. Mulai percakapan baru.",
	}
	if msg, ok := msgs[code]; ok {
		return "⚠️ " + msg
	}
	return "⚠️ Terjadi kesalahan. Silakan coba lagi atau ketik MENU."
}

// ─── Formatting Helpers ───────────────────────────────────────────────────────

// formatIDR formats a float64 as Indonesian Rupiah with thousand separators.
func formatIDR(amount float64) string {
	intPart := int64(amount)
	s := fmt.Sprintf("%d", intPart)
	// Insert thousand separators
	result := []byte(s)
	n := len(result)
	if n <= 3 {
		return s
	}
	var out []byte
	for i, ch := range result {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, ch)
	}
	return string(out)
}

// capitalise uppercases the first letter of a string.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}