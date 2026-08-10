package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

type CategoryRepository struct {
	db *sqlx.DB
}

func NewCategoryRepository(db *sqlx.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) Create(ctx context.Context, c *model.Category) error {
	const q = `
		INSERT INTO categories (name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q, c.Name, c.Slug, c.Description).
		Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *CategoryRepository) GetAll(ctx context.Context) ([]model.Category, error) {
	const q = `
		SELECT id, name, slug, description, created_at, updated_at
		FROM categories
		ORDER BY name ASC`
	var categories []model.Category
	err := r.db.SelectContext(ctx, &categories, q)
	return categories, err
}

func (r *CategoryRepository) GetByID(ctx context.Context, id int64) (*model.Category, error) {
	const q = `
		SELECT id, name, slug, description, created_at, updated_at
		FROM categories WHERE id = $1`
	var c model.Category
	if err := r.db.GetContext(ctx, &c, q, id); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CategoryRepository) Update(ctx context.Context, c *model.Category) error {
	const q = `
		UPDATE categories
		SET name = $1, slug = $2, description = $3, updated_at = NOW()
		WHERE id = $4`
	res, err := r.db.ExecContext(ctx, q, c.Name, c.Slug, c.Description, c.ID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("category not found")
	}
	return nil
}

func (r *CategoryRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM categories WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("category not found")
	}
	return nil
}

type CategoryFilter struct {
	Q     string
	Page  int
	Limit int
}

func (r *CategoryRepository) FindFiltered(ctx context.Context, filter CategoryFilter) ([]model.Category, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filter.Q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR slug ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+filter.Q+"%")
		argIdx++
	}

	whereStmt := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM categories WHERE %s", whereStmt)
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered count: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, name, slug, description, created_at, updated_at
		FROM categories
		WHERE %s
		ORDER BY name ASC`, whereStmt)

	if filter.Limit > 0 {
		if filter.Page <= 0 {
			filter.Page = 1
		}
		offset := (filter.Page - 1) * filter.Limit
		selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, filter.Limit, offset)
	}

	var categories []model.Category
	if err := r.db.SelectContext(ctx, &categories, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered select: %w", err)
	}

	return categories, total, nil
}
