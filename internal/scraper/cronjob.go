// internal/scraper/cronjob.go
package scraper

import (
	"context"
	"fmt"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

type PriceCronJob struct {
	scrapers    []Scraper
	productRepo *repository.ProductRepository
	priceRepo   *repository.PriceReferenceRepository
	logger      *zap.Logger
	interval    time.Duration
}

func NewPriceCronJob(
	productRepo *repository.ProductRepository,
	priceRepo *repository.PriceReferenceRepository,
	logger *zap.Logger,
) (*PriceCronJob, error) {
	shopeeScraper, err := NewShopeeScraper()
	if err != nil {
		return nil, fmt.Errorf("init shopee scraper: %w", err)
	}

	tokopediaScraper, err := NewTokopediaScraper()
	if err != nil {
		shopeeScraper.Close()
		return nil, fmt.Errorf("init tokopedia scraper: %w", err)
	}

	return &PriceCronJob{
		scrapers:    []Scraper{tokopediaScraper, shopeeScraper},
		productRepo: productRepo,
		priceRepo:   priceRepo,
		logger:      logger,
		interval:    14 * 24 * time.Hour,
	}, nil
}

func (c *PriceCronJob) Start(ctx context.Context) {
	go func() {
		c.run(ctx)
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.run(ctx)
			case <-ctx.Done():
				c.logger.Info("price cronjob stopped")
				return
			}
		}
	}()
}

func (c *PriceCronJob) run(ctx context.Context) {
	c.logger.Info("price scraper started")

	products, err := c.productRepo.GetAll(ctx)
	if err != nil {
		c.logger.Error("failed to load products", zap.Error(err))
		return
	}

	for _, product := range products {
		time.Sleep(2 * time.Second)

		for _, scraper := range c.scrapers {
			results, err := scraper.Search(ctx, product.Name)
			if err != nil {
				c.logger.Warn("scrape failed",
					zap.String("product", product.Name),
					zap.Error(err),
				)
				continue
			}

			for _, r := range results {
				if err := c.priceRepo.Upsert(ctx, model.PriceReference{
					ProductID:   product.ID,
					Marketplace: r.Marketplace,
					Price:       r.Price,
					URL:         r.URL,
					FetchedAt:   time.Now(),
				}); err != nil {
					c.logger.Error("upsert price ref failed", zap.Error(err))
				}
			}
		}
	}

	c.logger.Info("price scraper finished", zap.Int("products", len(products)))
}