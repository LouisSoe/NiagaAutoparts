package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ExcelProductRow satu baris data produk dari Excel
type ExcelProductRow struct {
	RowNumber   int
	SKU         string
	Name        string
	Category    string
	Description string
	Stock       int
	Price       float64
	Unit        string
	Location    string
	Error       string // jika ada error validasi di baris ini
}

// ExcelParseResult hasil parsing file Excel
type ExcelParseResult struct {
	SheetName   string
	TotalRows   int
	ValidRows   []ExcelProductRow
	InvalidRows []ExcelProductRow
	Headers     []string
}

// ParseExcel membaca file Excel dari bytes
// Mendukung format:
//   - Kolom: SKU | Nama | Kategori | Deskripsi | Stok | Harga | Satuan | Lokasi
//   - Header bisa di baris 1 (otomatis deteksi)
func ParseExcel(data []byte) (*ExcelParseResult, error) {
	f, err := excelize.OpenReader(strings.NewReader(string(data)))
	if err != nil {
		// coba dengan bytes langsung
		f, err = openExcelFromBytes(data)
		if err != nil {
			return nil, fmt.Errorf("excel open: %w", err)
		}
	}
	defer f.Close()

	// ambil sheet pertama
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("excel has no sheets")
	}
	sheetName := sheets[0]

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("excel get rows: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("excel sheet is empty")
	}

	result := &ExcelParseResult{SheetName: sheetName}

	// deteksi header row
	headerIdx, colMap := detectHeaders(rows)
	if headerIdx == -1 {
		return nil, fmt.Errorf("tidak ditemukan header yang dikenali. Pastikan ada kolom: SKU/Nama/Stok/Harga")
	}
	result.Headers = rows[headerIdx]

	// parse data rows (mulai setelah header)
	for i := headerIdx + 1; i < len(rows); i++ {
		row := rows[i]
		if isEmptyRow(row) {
			continue
		}

		result.TotalRows++
		parsed := parseProductRow(i+1, row, colMap)

		if parsed.Error != "" {
			result.InvalidRows = append(result.InvalidRows, parsed)
		} else {
			result.ValidRows = append(result.ValidRows, parsed)
		}
	}

	return result, nil
}

// detectHeaders cari baris header dan mapping kolom
// return (index baris header, map nama kolom → index kolom)
func detectHeaders(rows [][]string) (int, map[string]int) {
	// keyword yang dikenali per kolom
	colKeywords := map[string][]string{
		"sku":         {"sku", "kode", "kode produk", "product code"},
		"name":        {"nama", "name", "nama produk", "product name", "barang"},
		"category":    {"kategori", "category", "cat", "jenis"},
		"description": {"deskripsi", "description", "keterangan", "desc"},
		"stock":       {"stok", "stock", "qty", "quantity", "jumlah"},
		"price":       {"harga", "price", "harga jual", "harga beli"},
		"unit":        {"satuan", "unit", "uom"},
		"location":    {"lokasi", "location", "rak", "rack", "tempat"},
	}

	for rowIdx, row := range rows {
		colMap := map[string]int{}
		for colIdx, cell := range row {
			normalized := strings.ToLower(strings.TrimSpace(cell))
			for fieldName, keywords := range colKeywords {
				for _, kw := range keywords {
					if normalized == kw || strings.Contains(normalized, kw) {
						if _, exists := colMap[fieldName]; !exists {
							colMap[fieldName] = colIdx
						}
					}
				}
			}
		}
		// minimal harus ada kolom nama dan salah satu dari stok/harga
		_, hasName := colMap["name"]
		_, hasStock := colMap["stock"]
		_, hasPrice := colMap["price"]
		if hasName && (hasStock || hasPrice) {
			return rowIdx, colMap
		}
	}
	return -1, nil
}

// parseProductRow konversi satu row Excel ke struct
func parseProductRow(rowNum int, row []string, colMap map[string]int) ExcelProductRow {
	get := func(field string) string {
		idx, ok := colMap[field]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	p := ExcelProductRow{RowNumber: rowNum}
	p.SKU         = get("sku")
	p.Name        = get("name")
	p.Category    = get("category")
	p.Description = get("description")
	p.Unit        = get("unit")
	p.Location    = get("location")

	// parse stok
	if s := get("stock"); s != "" {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", "")
		v, err := strconv.Atoi(s)
		if err != nil {
			p.Error = fmt.Sprintf("baris %d: stok tidak valid '%s'", rowNum, get("stock"))
			return p
		}
		p.Stock = v
	}

	// parse harga
	if s := get("price"); s != "" {
		s = strings.ReplaceAll(s, "Rp", "")
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
		s = strings.TrimSpace(s)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			p.Error = fmt.Sprintf("baris %d: harga tidak valid '%s'", rowNum, get("price"))
			return p
		}
		p.Price = v
	}

	// validasi minimal
	if p.Name == "" {
		p.Error = fmt.Sprintf("baris %d: nama produk kosong", rowNum)
	}

	return p
}

func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// openExcelFromBytes helper untuk buka Excel dari []byte
func openExcelFromBytes(data []byte) (*excelize.File, error) {
	// excelize v2 support io.Reader
	reader := strings.NewReader(string(data))
	_ = reader
	// fallback: tulis ke temp dan buka
	return nil, fmt.Errorf("use excelize.OpenReader with bytes.NewReader")
}

// FormatExcelSummary ringkasan hasil parsing untuk WhatsApp
func FormatExcelSummary(result *ExcelParseResult) string {
	var sb strings.Builder

	sb.WriteString("📊 *File Excel Terdeteksi*\n\n")
	sb.WriteString(fmt.Sprintf("📋 Sheet: %s\n", result.SheetName))
	sb.WriteString(fmt.Sprintf("📦 Total baris data: %d\n", result.TotalRows))
	sb.WriteString(fmt.Sprintf("✅ Valid: %d baris\n", len(result.ValidRows)))

	if len(result.InvalidRows) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ Bermasalah: %d baris\n", len(result.InvalidRows)))
	}

	if len(result.ValidRows) > 0 {
		sb.WriteString("\n*Preview (5 data pertama):*\n")
		limit := 5
		if len(result.ValidRows) < limit {
			limit = len(result.ValidRows)
		}
		for _, row := range result.ValidRows[:limit] {
			sku := row.SKU
			if sku == "" {
				sku = "-"
			}
			sb.WriteString(fmt.Sprintf(
				"• %s | %s | Stok: %d | Rp %s\n",
				sku, row.Name, row.Stock, formatNumber(row.Price),
			))
		}
	}

	if len(result.InvalidRows) > 0 {
		sb.WriteString("\n*Baris bermasalah:*\n")
		for _, row := range result.InvalidRows {
			sb.WriteString(fmt.Sprintf("⚠️ %s\n", row.Error))
		}
	}

	if len(result.ValidRows) > 0 {
		sb.WriteString(fmt.Sprintf(
			"\nKetik *IMPORT KONFIRMASI* untuk update %d produk ke database.",
			len(result.ValidRows),
		))
	}

	return sb.String()
}