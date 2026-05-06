// internal/scraper/shopee.go
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

type ShopeeScraper struct {
	browser *rod.Browser
}

func NewShopeeScraper() (*ShopeeScraper, error) {
	path, found := launcher.LookPath()
	if !found {
		path = defaultChromePath()
	}
	if path == "" {
		return nil, fmt.Errorf("chrome not found")
	}

	u, err := launcher.New().
		Bin(path).
		Headless(true).
		Leakless(false).
		Launch()
	if err != nil {
		return nil, fmt.Errorf("shopee browser launch: %w", err)
	}

	browser := rod.New().ControlURL(u).MustConnect()
	return &ShopeeScraper{browser: browser}, nil
}

func (s *ShopeeScraper) Close() {
	if s.browser != nil {
		s.browser.MustClose()
	}
}

func (s *ShopeeScraper) Search(ctx context.Context, productName string) ([]PriceResult, error) {
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return nil, fmt.Errorf("shopee new page: %w", err)
	}
	defer page.MustClose()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	page = page.Context(timeoutCtx)

	if err := page.Navigate(fmt.Sprintf(
		"https://shopee.co.id/search?keyword=%s",
		strings.ReplaceAll(productName, " ", "%20"),
	)); err != nil {
		return nil, fmt.Errorf("shopee navigate: %w", err)
	}

	if err := page.WaitElementsMoreThan(`div[data-sqe="item"]`, 0); err != nil {
		return nil, fmt.Errorf("shopee wait elements: %w", err)
	}

	elements, err := page.Elements(`div[data-sqe="item"]`)
	if err != nil {
		return nil, fmt.Errorf("shopee elements: %w", err)
	}

	results := []PriceResult{}
	for i, el := range elements {
		if i >= 5 {
			break
		}

		nameEl, err := el.Element(`div._10Wbs- span`)
		if err != nil {
			continue
		}
		name, _ := nameEl.Text()

		priceEl, err := el.Element(`span._341bF7`)
		if err != nil {
			continue
		}
		priceStr, _ := priceEl.Text()

		linkEl, err := el.Element(`a`)
		if err != nil {
			continue
		}
		href, _ := linkEl.Attribute("href")

		clean := strings.ReplaceAll(priceStr, "Rp", "")
		clean = strings.ReplaceAll(clean, ".", "")
		clean = strings.TrimSpace(clean)

		price, err := strconv.ParseFloat(clean, 64)
		if err != nil || price == 0 || name == "" {
			continue
		}

		url := ""
		if href != nil {
			url = *href
		}

		results = append(results, PriceResult{
			Name:        name, // FIX Bug 6: field Name sekarang diisi
			Marketplace: "shopee",
			Price:       price,
			URL:         url,
		})
	}
	return results, nil
}