package utils

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
)

// SimplePDF represents a lightweight pure Go PDF 1.4 generator.
type SimplePDF struct {
	buf     bytes.Buffer
	objects []int
	content bytes.Buffer
	pages   int
}

func NewSimplePDF() *SimplePDF {
	pdf := &SimplePDF{}
	pdf.buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	pdf.pages = 1
	return pdf
}

func (p *SimplePDF) addObj(data string) int {
	offset := p.buf.Len()
	objNum := len(p.objects) + 1
	p.objects = append(p.objects, offset)
	fmt.Fprintf(&p.buf, "%d 0 obj\n%s\nendobj\n", objNum, data)
	return objNum
}

func sanitizePDFText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	return text
}

// GenerateSalesReportPDF writes a styled sales report PDF to the given writer.
func GenerateSalesReportPDF(w io.Writer, title, periodStr string, summaryKeys []string, summaryVals []string, headers []string, rows [][]string) error {
	var stream bytes.Buffer

	// Title
	fmt.Fprintf(&stream, "BT /F2 16 Tf 0 0 0 rg 40 800 Td (%s) Tj ET\n", sanitizePDFText(title))

	// Period / Subtitle
	fmt.Fprintf(&stream, "BT /F1 10 Tf 0.3 0.3 0.3 rg 40 782 Td (%s) Tj ET\n", sanitizePDFText(periodStr))

	// Divider line
	stream.WriteString("0.8 0.8 0.8 RG 0.75 w 40 770 m 555 770 l S\n")

	// Summary Box (background rectangle)
	stream.WriteString("0.95 0.95 0.98 rg 40 705 515 52 re f\n")
	stream.WriteString("0.8 0.8 0.85 RG 0.5 w 40 705 515 52 re S\n")

	stream.WriteString("BT /F2 10 Tf 0.2 0.2 0.5 rg 50 742 Td (RINGKASAN LAPORAN) Tj ET\n")
	
	startX := 50
	for i := 0; i < len(summaryKeys); i++ {
		key := summaryKeys[i]
		val := ""
		if i < len(summaryVals) {
			val = summaryVals[i]
		}
		x := startX + (i * 165)
		fmt.Fprintf(&stream, "BT /F1 9 Tf 0.4 0.4 0.4 rg %d 725 Td (%s:) Tj ET\n", x, sanitizePDFText(key))
		fmt.Fprintf(&stream, "BT /F2 10 Tf 0.1 0.1 0.1 rg %d 712 Td (%s) Tj ET\n", x, sanitizePDFText(val))
	}

	// Table Setup
	rowHeight := 20
	colWidths := []int{100, 100, 115, 65, 55, 80} // Total 515 width

	// Table Header Background
	stream.WriteString("0.2 0.35 0.6 rg 40 660 515 20 re f\n")

	currX := 40
	for i, h := range headers {
		fmt.Fprintf(&stream, "BT /F2 9 Tf 1 1 1 rg %d 666 Td (%s) Tj ET\n", currX+5, sanitizePDFText(h))
		if i < len(colWidths) {
			currX += colWidths[i]
		}
	}

	// Table Rows
	currY := 640
	for rIdx, row := range rows {
		if currY < 50 {
			break // Prevent overflowing page margin
		}

		// Alternating row background
		if rIdx%2 == 1 {
			fmt.Fprintf(&stream, "0.97 0.97 0.97 rg 40 %d 515 %d re f\n", currY, rowHeight)
		}

		// Row bottom border line
		fmt.Fprintf(&stream, "0.9 0.9 0.9 RG 0.5 w 40 %d 555 %d l S\n", currY, currY)

		currX = 40
		for cIdx, cell := range row {
			wCol := 70
			if cIdx < len(colWidths) {
				wCol = colWidths[cIdx]
			}
			// Truncate cell text if too long
			if len(cell) > 22 {
				cell = cell[:20] + ".."
			}
			fmt.Fprintf(&stream, "BT /F1 8.5 Tf 0.1 0.1 0.1 rg %d %d Td (%s) Tj ET\n", currX+5, currY+6, sanitizePDFText(cell))
			currX += wCol
		}
		currY -= rowHeight
	}

	// Footer
	footerStr := fmt.Sprintf("Dicetak pada: %s | NiagaGudang AutoParts Backend", time.Now().Format("02 Jan 2006 15:04 WIB"))
	fmt.Fprintf(&stream, "BT /F1 8 Tf 0.5 0.5 0.5 rg 40 30 Td (%s) Tj ET\n", sanitizePDFText(footerStr))

	// Assemble PDF Document Objects
	pdf := NewSimplePDF()

	// Obj 1: Catalog
	// Obj 2: Pages
	// Obj 3: Page 1
	// Obj 4: Font Helvetica
	// Obj 5: Font Helvetica-Bold
	// Obj 6: Contents Stream

	streamBytes := stream.Bytes()
	streamLen := len(streamBytes)

	pdf.addObj("<< /Type /Catalog /Pages 2 0 R >>")
	pdf.addObj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	pdf.addObj("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> /Contents 6 0 R >>")
	pdf.addObj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	pdf.addObj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")

	streamObjContent := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", streamLen, streamBytes)
	pdf.addObj(streamObjContent)

	// Write Xref
	xrefOffset := pdf.buf.Len()
	fmt.Fprintf(&pdf.buf, "xref\n0 %d\n0000000000 65535 f \n", len(pdf.objects)+1)
	for _, off := range pdf.objects {
		fmt.Fprintf(&pdf.buf, "%010d 00000 n \n", off)
	}

	// Write Trailer
	fmt.Fprintf(&pdf.buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(pdf.objects)+1, xrefOffset)

	_, err := w.Write(pdf.buf.Bytes())
	return err
}
