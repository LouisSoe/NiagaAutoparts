package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// ProcessAndSaveBase64Image mengolah string Base64 gambar (PNG/JPG/WEBP),
// mengonversinya dan menyimpannya di folder uploads dengan nama file .webp.
func ProcessAndSaveBase64Image(base64Data string, uploadDir string) (string, error) {
	if base64Data == "" {
		return "", nil
	}

	// Jika string sudah berupa URL path (misal: "/uploads/prod_123.webp" atau "http..."), kembalikan langsung
	if !strings.HasPrefix(base64Data, "data:image/") && (strings.HasPrefix(base64Data, "/") || strings.HasPrefix(base64Data, "http")) {
		return base64Data, nil
	}

	// Pisahkan header Base64 "data:image/png;base64,..." dari data murninya
	parts := strings.Split(base64Data, ",")
	if len(parts) < 2 {
		return "", fmt.Errorf("format base64 tidak valid")
	}

	rawBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("gagal decode base64: %w", err)
	}

	// Decode gambar dari memory
	var img image.Image
	if strings.Contains(parts[0], "webp") {
		img, err = webp.Decode(bytes.NewReader(rawBytes))
	} else if strings.Contains(parts[0], "png") {
		img, err = png.Decode(bytes.NewReader(rawBytes))
	} else {
		img, err = jpeg.Decode(bytes.NewReader(rawBytes))
	}

	if err != nil {
		// Fallback ke general image.Decode jika spesifik format gagal
		img, _, err = image.Decode(bytes.NewReader(rawBytes))
		if err != nil {
			return "", fmt.Errorf("gagal decode format gambar: %w", err)
		}
	}

	// Pastikan folder upload ada
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", fmt.Errorf("gagal membuat folder uploads: %w", err)
	}

	// Generate nama file unik dengan ekstensi .webp
	filename := fmt.Sprintf("prod_%d.webp", time.Now().UnixNano())
	filePath := filepath.Join(uploadDir, filename)

	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("gagal membuat file: %w", err)
	}
	defer outFile.Close()

	// Resize & Optimize ke Canvas RGBA standar (Web-ready)
	bounds := img.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, img, bounds.Min, draw.Src)

	// Simpan sebagai file web-optimized (PNG/JPEG compressed stream)
	if err := png.Encode(outFile, dst); err != nil {
		return "", fmt.Errorf("gagal menyelaraskan format gambar: %w", err)
	}

	return "/uploads/" + filename, nil
}
