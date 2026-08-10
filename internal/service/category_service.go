package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/louissoe/niaga-autoparts/internal/model"
	"github.com/louissoe/niaga-autoparts/internal/repository"
	"go.uber.org/zap"
)

type CategoryService struct {
	repo   *repository.CategoryRepository
	logger *zap.Logger
}

func NewCategoryService(repo *repository.CategoryRepository, logger *zap.Logger) *CategoryService {
	return &CategoryService{
		repo:   repo,
		logger: logger,
	}
}

func (s *CategoryService) CreateCategory(ctx context.Context, c *model.Category) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return fmt.Errorf("nama kategori tidak boleh kosong")
	}
	if c.Slug == "" {
		c.Slug = strings.ToLower(strings.ReplaceAll(c.Name, " ", "-"))
	}
	return s.repo.Create(ctx, c)
}

func (s *CategoryService) GetAllCategories(ctx context.Context) ([]model.Category, error) {
	return s.repo.GetAll(ctx)
}

func (s *CategoryService) GetCategoryByID(ctx context.Context, id int64) (*model.Category, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *CategoryService) UpdateCategory(ctx context.Context, c *model.Category) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return fmt.Errorf("nama kategori tidak boleh kosong")
	}
	if c.Slug == "" {
		c.Slug = strings.ToLower(strings.ReplaceAll(c.Name, " ", "-"))
	}
	return s.repo.Update(ctx, c)
}

func (s *CategoryService) DeleteCategory(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *CategoryService) GetFilteredCategories(ctx context.Context, filter repository.CategoryFilter) ([]model.Category, int64, error) {
	return s.repo.FindFiltered(ctx, filter)
}
