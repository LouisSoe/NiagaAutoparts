package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	const q = `
		INSERT INTO users (email, password_hash, name, role, phone, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q, u.Email, u.PasswordHash, u.Name, u.Role, u.Phone, u.IsActive).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (r *UserRepository) GetAll(ctx context.Context) ([]model.User, error) {
	const q = `
		SELECT id, email, password_hash, name, role, phone, is_active, created_at, updated_at
		FROM users
		ORDER BY created_at DESC`
	var users []model.User
	err := r.db.SelectContext(ctx, &users, q)
	return users, err
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	const q = `
		SELECT id, email, password_hash, name, role, phone, is_active, created_at, updated_at
		FROM users WHERE id = $1`
	var u model.User
	if err := r.db.GetContext(ctx, &u, q, id); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	const q = `
		SELECT id, email, password_hash, name, role, phone, is_active, created_at, updated_at
		FROM users WHERE email = $1`
	var u model.User
	if err := r.db.GetContext(ctx, &u, q, email); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) Update(ctx context.Context, u *model.User) error {
	const q = `
		UPDATE users
		SET email = $1, password_hash = $2, name = $3, role = $4, phone = $5, is_active = $6, updated_at = NOW()
		WHERE id = $7`
	res, err := r.db.ExecContext(ctx, q, u.Email, u.PasswordHash, u.Name, u.Role, u.Phone, u.IsActive, u.ID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM users WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

type UserFilter struct {
	Q        string
	Role     string
	IsActive *bool
	Page     int
	Limit    int
}

func (r *UserRepository) FindFiltered(ctx context.Context, filter UserFilter) ([]model.User, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if filter.Q != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+filter.Q+"%")
		argIdx++
	}

	if filter.Role != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(role) = $%d", argIdx))
		args = append(args, strings.ToLower(filter.Role))
		argIdx++
	}

	if filter.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *filter.IsActive)
		argIdx++
	}

	whereStmt := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users WHERE %s", whereStmt)
	var total int64
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered count: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, email, password_hash, name, role, phone, is_active, created_at, updated_at
		FROM users
		WHERE %s
		ORDER BY created_at DESC`, whereStmt)

	if filter.Limit > 0 {
		if filter.Page <= 0 {
			filter.Page = 1
		}
		offset := (filter.Page - 1) * filter.Limit
		selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, filter.Limit, offset)
	}

	var users []model.User
	if err := r.db.SelectContext(ctx, &users, selectQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("FindFiltered select: %w", err)
	}

	return users, total, nil
}
