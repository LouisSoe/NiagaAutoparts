// internal/scraper/tokopedia.go
package scraper

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type TokopediaScraper struct {
	browser *rod.Browser // FIX Bug 5: persistent browser, konsisten dengan ShopeeScraper
}

// FIX Bug 2 & 3: return (*TokopediaScraper, error) agar cocok dengan pemanggilan di cronjob.go
func NewTokopediaScraper() (*TokopediaScraper, error) {
	path, found := launcher.LookPath()
	if !found {
		path = defaultChromePath()
	}
	if path == "" {
		return nil, fmt.Errorf("chrome not found")
	}

	// FIX Bug 4: ganti MustLaunch() → Launch() agar error bisa di-handle, bukan panic
	u, err := launcher.New().
		Bin(path).
		Headless(true).
		NoSandbox(true).
		Leakless(false).
		Launch()
	if err != nil {
		return nil, fmt.Errorf("tokopedia browser launch: %w", err)
	}

	browser := rod.New().ControlURL(u).MustConnect()
	return &TokopediaScraper{browser: browser}, nil
}

// FIX Bug 3: tambahkan method Close() agar TokopediaScraper bisa di-cleanup
func (s *TokopediaScraper) Close() {
	if s.browser != nil {
		s.browser.MustClose()
	}
}

func (s *TokopediaScraper) Search(ctx context.Context, productName string) ([]PriceResult, error) {
	// FIX Bug 4: ganti MustPage() → Page() agar error bisa di-handle, bukan panic
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return nil, fmt.Errorf("tokopedia new page: %w", err)
	}
	defer page.MustClose()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	page = page.Context(timeoutCtx)

	if err := page.Navigate(fmt.Sprintf(
		"https://www.tokopedia.com/search?st=product&q=%s",
		strings.ReplaceAll(productName, " ", "+"),
	)); err != nil {
		return nil, fmt.Errorf("tokopedia navigate: %w", err)
	}

	if err := page.WaitElementsMoreThan(`div[data-testid="master-product-card"]`, 0); err != nil {
		return nil, fmt.Errorf("tokopedia wait elements: %w", err)
	}

	elements, err := page.Elements(`div[data-testid="master-product-card"]`)
	if err != nil {
		return nil, fmt.Errorf("tokopedia elements: %w", err)
	}

	results := []PriceResult{}
	for i, el := range elements {
		if i >= 5 {
			break
		}

		nameEl, err := el.Element(`span[data-testid="spnSRPProdName"]`)
		if err != nil {
			continue
		}
		name, _ := nameEl.Text()

		priceEl, err := el.Element(`span[data-testid="spnSRPProdPrice"]`)
		if err != nil {
			continue
		}
		priceStr, _ := priceEl.Text()

		linkEl, err := el.Element(`a`)
		if err != nil {
			continue
		}
		href, _ := linkEl.Attribute("href")

		// bersihkan harga: "Rp25.000" → 25000
		clean := strings.ReplaceAll(priceStr, "Rp", "")
		clean = strings.ReplaceAll(clean, ".", "")
		clean = strings.ReplaceAll(clean, ",", "")
		clean = strings.TrimSpace(clean)

		price, err := strconv.ParseFloat(clean, 64)
		if err != nil || price == 0 {
			continue
		}

		url := ""
		if href != nil {
			url = *href
		}

		results = append(results, PriceResult{
			Name:        name,
			Marketplace: "tokopedia",
			Price:       price,
			URL:         url,
		})
	}
	return results, nil
}