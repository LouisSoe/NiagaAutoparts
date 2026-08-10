package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/louissoe/niaga-autoparts/internal/cache"
	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"github.com/louissoe/niaga-autoparts/internal/utils"
	"go.uber.org/zap"
)

// ProductService handles product search and price lookup.
// Search results are cached in-memory to avoid repeated DB hits for the same
// keyword within the TTL window (default 60 s).
type ProductService struct {
	productRepo *repository.ProductRepository
	cache       *cache.Cache
	productTTL  time.Duration
	logger      *zap.Logger
    dictionary   []string
}

func NewProductService(
	repo *repository.ProductRepository,
	c *cache.Cache,
	productTTL time.Duration,
	logger *zap.Logger,
) *ProductService {
	return &ProductService{
		productRepo: repo,
		cache:       c,
		productTTL:  productTTL,
		logger:      logger,
	}
}

// Search queries the database for matching products, with an in-memory cache
// layer so identical queries within the TTL don't hit MySQL.
func (s *ProductService) Search(ctx context.Context, query string) ([]model.Product, error) {
    // ── Normalisasi ────────────────────────────────────────────────────────
    query = strings.TrimSpace(query)
    if query == "" {
        return nil, fmt.Errorf("empty search query")
    }

    // normalisasi: lowercase + collapse spasi ganda
    normalized := strings.ToLower(query)
    normalized = strings.Join(strings.Fields(normalized), " ")

    // pecah jadi words, buang kata < 2 char
    words := []string{}
    for _, w := range strings.Fields(normalized) {
        if len([]rune(w)) >= 2 {
            words = append(words, w)
        }
    }
    if len(words) == 0 {
        return nil, fmt.Errorf("query too short")
    }

    // koreksi typo per kata
    corrected := make([]string, len(words))
    for i, w := range words {
        corrected[i] = utils.CorrectWord(w, s.dictionary)
    }

    s.logger.Info("product search",
        zap.Strings("original", words),
        zap.Strings("corrected", corrected),
    )

    // cache key dari normalized query, bukan raw
    cacheKey := "product_search:" + normalized

    // ── Cache hit ──────────────────────────────────────────────────────────
    if raw, ok := s.cache.Get(ctx, cacheKey); ok {
        var products []model.Product
        if err := json.Unmarshal([]byte(raw), &products); err == nil {
            s.logger.Debug("product search cache hit",
                zap.String("query", normalized),
                zap.Int("words", len(words)),
            )
            return products, nil
        }
    }

    // ── Cache miss → DB ────────────────────────────────────────────────────
    products, err := s.productRepo.Search(ctx, corrected)
    s.logger.Info("product search called",
        zap.String("raw_query", query),
        zap.Strings("words", words),
        zap.Int("result_count", len(products)),
    )
    if err != nil {
        s.logger.Error("product search failed",
            zap.String("query", normalized),
            zap.Strings("words", words),
            zap.Error(err),
        )
        return nil, err
    }

    // ── Populate cache ─────────────────────────────────────────────────────
    if b, err := json.Marshal(products); err == nil {
        s.cache.Set(ctx, cacheKey, string(b), s.productTTL)
    }

    return products, nil
}

// GetWithPriceRefs returns a product with its marketplace price references.
// Product detail is cached separately from the search list.
func (s *ProductService) GetWithPriceRefs(ctx context.Context, productID int64) (*model.Product, []model.PriceReference, error) {
	product, err := s.productRepo.GetByID(ctx, productID)
	if err != nil {
		return nil, nil, err
	}
	refs, err := s.productRepo.GetPriceReferences(ctx, productID)
	if err != nil {
		s.logger.Warn("failed to get price references",
			zap.Int64("product_id", productID), zap.Error(err))
		return product, nil, nil // non-fatal
	}
	return product, refs, nil
}

// GetAll fetches every active product — used as the data source for
// in-memory linear search when the LIKE-based DB query yields no results.
func (s *ProductService) GetAll(ctx context.Context) ([]model.Product, error) {
	return s.productRepo.GetAll(ctx)
}

// InvalidateSearchCache drops the cached results for a query (e.g. after
// stock changes). Pass the same query string that was used to search.
func (s *ProductService) InvalidateSearchCache(ctx context.Context, query string) {
	key := "product_search:" + strings.ToLower(strings.TrimSpace(query))
	s.cache.Delete(ctx, key)
}
// di ProductService, panggil saat init
func (s *ProductService) BuildDictionary(ctx context.Context) error {
    dict, err := s.productRepo.GetAllTokens(ctx)
    if err != nil {
        return err
    }
    s.dictionary = dict
    s.logger.Info("dictionary built", zap.Int("words", len(dict)))
    return nil
}

// GetByID retrieves a single product by ID.
func (s *ProductService) GetByID(ctx context.Context, id int64) (*model.Product, error) {
	return s.productRepo.GetByID(ctx, id)
}

// CreateProduct creates a new product in DB.
func (s *ProductService) CreateProduct(ctx context.Context, p *model.Product) error {
	if p.Name == "" {
		return fmt.Errorf("nama produk wajib diisi")
	}
	if p.SKU == "" {
		p.SKU = repository.GenerateSKU(p.Name, p.CategoryName)
	}

	// Olah gambar jika dikirim dalam format Base64 dari frontend
	if p.ImageURL.Valid && p.ImageURL.String != "" {
		savedPath, err := utils.ProcessAndSaveBase64Image(p.ImageURL.String, "uploads")
		if err != nil {
			s.logger.Warn("gagal memproses gambar produk", zap.Error(err))
			p.ImageURL.String = ""
			p.ImageURL.Valid = false
		} else if savedPath != "" {
			p.ImageURL.String = savedPath
			p.ImageURL.Valid = true
		}
	}

	return s.productRepo.Create(ctx, p)
}

// UpdateProduct updates an existing product in DB.
func (s *ProductService) UpdateProduct(ctx context.Context, p *model.Product) error {
	if p.ID <= 0 {
		return fmt.Errorf("invalid product ID")
	}

	// Olah gambar jika dikirim dalam format Base64 dari frontend
	if p.ImageURL.Valid && p.ImageURL.String != "" {
		savedPath, err := utils.ProcessAndSaveBase64Image(p.ImageURL.String, "uploads")
		if err != nil {
			s.logger.Warn("gagal memproses gambar produk", zap.Error(err))
			p.ImageURL.String = ""
			p.ImageURL.Valid = false
		} else if savedPath != "" {
			p.ImageURL.String = savedPath
			p.ImageURL.Valid = true
		}
	}

	return s.productRepo.Update(ctx, p)
}

// DeleteProduct soft deletes a product by ID.
func (s *ProductService) DeleteProduct(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid product ID")
	}
	return s.productRepo.Delete(ctx, id)
}

// UpsertFromExcel delegasi ke repository untuk import produk
func (s *ProductService) UpsertFromExcel(ctx context.Context, input repository.ExcelProductInput) error {
	return s.productRepo.UpsertFromExcel(ctx, input)
}

func (s *ProductService) GetFilteredProducts(ctx context.Context, filter repository.ProductFilter) ([]model.Product, int64, error) {
	return s.productRepo.FindFiltered(ctx, filter)
}