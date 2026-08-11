package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/utils"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

type ReportService struct {
	repo   *repository.ReportRepository
	logger *zap.Logger
}

func NewReportService(repo *repository.ReportRepository, logger *zap.Logger) *ReportService {
	return &ReportService{
		repo:   repo,
		logger: logger,
	}
}

func (s *ReportService) GetSalesReport(ctx context.Context, filter model.SalesReportFilter) (*model.SalesReportData, int64, error) {
	return s.repo.GetSalesReport(ctx, filter)
}

func (s *ReportService) ExportSalesReportCSV(ctx context.Context, w io.Writer, filter model.SalesReportFilter) error {
	// Fetch all orders matching filter without pagination limit
	filter.Limit = 0
	filter.Page = 0

	data, _, err := s.repo.GetSalesReport(ctx, filter)
	if err != nil {
		s.logger.Error("failed to get sales report data for export", zap.Error(err))
		return err
	}

	// Write UTF-8 BOM for Microsoft Excel compatibility
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header row
	header := []string{
		"No. Transaksi",
		"Tanggal & Waktu",
		"Pelanggan",
		"Status Pesanan",
		"Metode Pembayaran",
		"Sumber",
		"Jumlah Items",
		"Total Harga (IDR)",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Data rows
	for _, row := range data.Orders {
		record := []string{
			row.OrderNumber,
			row.CreatedAt.Format("2006-01-02 15:04:05"),
			row.CustomerName,
			row.Status,
			row.PaymentMethod,
			row.Source,
			strconv.Itoa(row.ItemCount),
			fmt.Sprintf("%.2f", row.TotalPrice),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	// Summary row at the bottom
	writer.Write([]string{}) // Empty separator row
	summaryHeader := []string{"Ringkasan Laporan Penjualan"}
	writer.Write(summaryHeader)
	writer.Write([]string{"Total Transaksi", strconv.FormatInt(data.Summary.TotalOrders, 10)})
	writer.Write([]string{"Total Pendapatan (Lunas)", fmt.Sprintf("%.2f", data.Summary.TotalRevenue)})
	writer.Write([]string{"Total Modal/HPP", fmt.Sprintf("%.2f", data.Summary.TotalCost)})
	writer.Write([]string{"Total Keuntungan Bersih", fmt.Sprintf("%.2f", data.Summary.TotalProfit)})
	writer.Write([]string{"Margin Keuntungan (%)", fmt.Sprintf("%.2f%%", data.Summary.ProfitMargin)})
	writer.Write([]string{"Total Items Terjual", strconv.FormatInt(data.Summary.TotalItems, 10)})

	return nil
}

func (s *ReportService) ExportStockReportCSV(ctx context.Context, w io.Writer, categoryID int64, lowStockOnly bool) error {
	rows, err := s.repo.GetStockReport(ctx, categoryID, lowStockOnly)
	if err != nil {
		s.logger.Error("failed to get stock report data for export", zap.Error(err))
		return err
	}

	// Write UTF-8 BOM for Excel compatibility
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header row
	header := []string{
		"SKU",
		"Nama Produk",
		"Kategori",
		"Lokasi Rak",
		"Stok Total",
		"Stok Reservasi",
		"Stok Tersedia",
		"Stok Minimal",
		"Satuan",
		"Harga Beli (IDR)",
		"Harga Jual (IDR)",
		"Status Stok",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range rows {
		record := []string{
			row.SKU,
			row.Name,
			row.Category,
			row.Location,
			strconv.Itoa(row.Stock),
			strconv.Itoa(row.Reserved),
			strconv.Itoa(row.Available),
			strconv.Itoa(row.MinimumStock),
			row.Unit,
			fmt.Sprintf("%.2f", row.BuyPrice),
			fmt.Sprintf("%.2f", row.SellPrice),
			row.StatusLabel,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func (s *ReportService) ExportSalesReportPDF(ctx context.Context, w io.Writer, filter model.SalesReportFilter) error {
	filter.Limit = 0
	filter.Page = 0

	data, _, err := s.repo.GetSalesReport(ctx, filter)
	if err != nil {
		s.logger.Error("failed to get sales report data for PDF export", zap.Error(err))
		return err
	}

	periodStr := "Periode: Semua Tanggal"
	if filter.StartDate != "" || filter.EndDate != "" {
		periodStr = fmt.Sprintf("Periode: %s s.d. %s", filter.StartDate, filter.EndDate)
	}
	if filter.Status != "" && filter.Status != "all" {
		periodStr += fmt.Sprintf(" | Status: %s", filter.Status)
	}

	summaryKeys := []string{"Total Transaksi", "Items Terjual", "Total Omzet", "Total Modal (HPP)", "Keuntungan Bersih", "Margin (%)"}
	summaryVals := []string{
		strconv.FormatInt(data.Summary.TotalOrders, 10),
		strconv.FormatInt(data.Summary.TotalItems, 10),
		fmt.Sprintf("Rp %.0f", data.Summary.TotalRevenue),
		fmt.Sprintf("Rp %.0f", data.Summary.TotalCost),
		fmt.Sprintf("Rp %.0f", data.Summary.TotalProfit),
		fmt.Sprintf("%.1f%%", data.Summary.ProfitMargin),
	}

	headers := []string{"No. Order", "Tanggal", "Pelanggan", "Metode", "Status", "Total (Rp)"}
	rows := make([][]string, 0, len(data.Orders))
	for _, item := range data.Orders {
		rows = append(rows, []string{
			item.OrderNumber,
			item.CreatedAt.Format("2006-01-02"),
			item.CustomerName,
			item.PaymentMethod,
			item.Status,
			fmt.Sprintf("%.0f", item.TotalPrice),
		})
	}

	return utils.GenerateSalesReportPDF(w, "LAPORAN PENJUALAN - NIAGA GUDANG", periodStr, summaryKeys, summaryVals, headers, rows)
}

func (s *ReportService) ExportStockReportPDF(ctx context.Context, w io.Writer, categoryID int64, lowStockOnly bool) error {
	rows, err := s.repo.GetStockReport(ctx, categoryID, lowStockOnly)
	if err != nil {
		s.logger.Error("failed to get stock report data for PDF export", zap.Error(err))
		return err
	}

	periodStr := "Kategori: Semua Produk"
	if lowStockOnly {
		periodStr += " | Filter: Stok Menipis/Habis Only"
	}

	totalItems := len(rows)
	outOfStock := 0
	for _, r := range rows {
		if r.Available <= 0 {
			outOfStock++
		}
	}

	summaryKeys := []string{"Total Produk", "Stok Habis"}
	summaryVals := []string{
		strconv.Itoa(totalItems),
		strconv.Itoa(outOfStock),
	}

	headers := []string{"SKU", "Nama Produk", "Kategori", "Lokasi", "Stok", "Harga (Rp)"}
	tableRows := make([][]string, 0, len(rows))
	for _, item := range rows {
		tableRows = append(tableRows, []string{
			item.SKU,
			item.Name,
			item.Category,
			item.Location,
			fmt.Sprintf("%d (%s)", item.Available, item.StatusLabel),
			fmt.Sprintf("%.0f", item.SellPrice),
		})
	}

	return utils.GenerateSalesReportPDF(w, "LAPORAN STOK GUDANG - NIAGA GUDANG", periodStr, summaryKeys, summaryVals, headers, tableRows)
}

func (s *ReportService) ExportSalesReportExcel(ctx context.Context, w io.Writer, filter model.SalesReportFilter) error {
	filter.Limit = 0
	filter.Page = 0

	data, _, err := s.repo.GetSalesReport(ctx, filter)
	if err != nil {
		s.logger.Error("failed to get sales report data for Excel export", zap.Error(err))
		return err
	}

	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Laporan Penjualan"
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)
	_ = f.DeleteSheet("Sheet1")

	f.SetCellValue(sheetName, "A1", "LAPORAN PENJUALAN - NIAGA GUDANG AUTOPARTS")

	headers := []string{"No. Transaksi", "Tanggal & Waktu", "Pelanggan", "Status Pesanan", "Metode Pembayaran", "Sumber", "Jumlah Items", "Total Harga (IDR)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		f.SetCellValue(sheetName, cell, h)
	}

	rowIdx := 4
	for _, row := range data.Orders {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), row.OrderNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), row.CreatedAt.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), row.CustomerName)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), row.Status)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), row.PaymentMethod)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowIdx), row.Source)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", rowIdx), row.ItemCount)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", rowIdx), row.TotalPrice)
		rowIdx++
	}

	rowIdx += 1
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "RINGKASAN PENJUALAN")
	rowIdx++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "Total Transaksi")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), data.Summary.TotalOrders)
	rowIdx++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "Total Pendapatan (Lunas)")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), data.Summary.TotalRevenue)
	rowIdx++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "Total Modal (HPP)")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), data.Summary.TotalCost)
	rowIdx++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "Total Keuntungan Bersih")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), data.Summary.TotalProfit)
	rowIdx++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "Margin Keuntungan (%)")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), fmt.Sprintf("%.2f%%", data.Summary.ProfitMargin))
	rowIdx++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), "Total Items Terjual")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), data.Summary.TotalItems)

	_, err = f.WriteTo(w)
	return err
}

func (s *ReportService) ExportStockReportExcel(ctx context.Context, w io.Writer, categoryID int64, lowStockOnly bool) error {
	rows, err := s.repo.GetStockReport(ctx, categoryID, lowStockOnly)
	if err != nil {
		s.logger.Error("failed to get stock report data for Excel export", zap.Error(err))
		return err
	}

	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Stok Produk Gudang"
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)
	_ = f.DeleteSheet("Sheet1")

	f.SetCellValue(sheetName, "A1", "LAPORAN STOK PRODUK GUDANG - NIAGA GUDANG")

	headers := []string{"SKU", "Nama Produk", "Kategori", "Lokasi Rak", "Stok Total", "Stok Reservasi", "Stok Tersedia", "Stok Minimal", "Satuan", "Harga Beli (IDR)", "Harga Jual (IDR)", "Status Stok"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		f.SetCellValue(sheetName, cell, h)
	}

	rowIdx := 4
	for _, row := range rows {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), row.SKU)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), row.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), row.Category)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), row.Location)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), row.Stock)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowIdx), row.Reserved)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", rowIdx), row.Available)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", rowIdx), row.MinimumStock)
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", rowIdx), row.Unit)
		f.SetCellValue(sheetName, fmt.Sprintf("J%d", rowIdx), row.BuyPrice)
		f.SetCellValue(sheetName, fmt.Sprintf("K%d", rowIdx), row.SellPrice)
		f.SetCellValue(sheetName, fmt.Sprintf("L%d", rowIdx), row.StatusLabel)
		rowIdx++
	}

	_, err = f.WriteTo(w)
	return err
}

func (s *ReportService) GenerateUserOrdersExcel(orders []model.Order, year, month int, w io.Writer) error {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := fmt.Sprintf("Riwayat %02d-%d", month, year)
	index, _ := f.NewSheet(sheetName)
	f.SetActiveSheet(index)
	_ = f.DeleteSheet("Sheet1")

	f.SetCellValue(sheetName, "A1", fmt.Sprintf("LAPORAN RIWAYAT PESANAN BULAN %02d/%d", month, year))

	headers := []string{"No.", "No. Pesanan", "Tanggal", "Item Produk", "Total Jumlah", "Total Harga (IDR)", "Status", "Metode Pembayaran"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		f.SetCellValue(sheetName, cell, h)
	}

	rowIdx := 4
	for i, order := range orders {
		prodSummary := "-"
		totalQty := 0
		if len(order.Items) > 0 {
			var prods []string
			for _, item := range order.Items {
				prods = append(prods, fmt.Sprintf("%s (%d pcs)", item.ProductName, item.Quantity))
				totalQty += item.Quantity
			}
			prodSummary = strings.Join(prods, ", ")
		}

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), i+1)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), order.OrderNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), order.CreatedAt.Format("02-01-2006 15:04"))
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), prodSummary)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), totalQty)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowIdx), order.TotalPrice)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", rowIdx), order.Status)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", rowIdx), order.PaymentMethod.String)
		rowIdx++
	}

	_, err := f.WriteTo(w)
	return err
}
